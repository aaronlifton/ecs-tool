package main

import (
  "fmt"
  "strings"
  "net/http"
  "io/ioutil"
  "encoding/json"
  "github.com/aws/aws-sdk-go/service/ecs"
  "github.com/aws/aws-sdk-go/service/ec2"
  "github.com/aws/aws-sdk-go/aws"
  "github.com/aws/aws-sdk-go/aws/awserr"
  "github.com/aws/aws-sdk-go/aws/session"
  "github.com/aws/aws-sdk-go/aws/endpoints"
  "os"
  "os/exec"
  "log"
  "errors"
  "io"
  "path/filepath"
)

type EcsToolConfig struct {
  SshUser string
  KeyPath string
  AlwaysEnterContainer bool
}

func readConfig(path string) (EcsToolConfig, error) {
  data, err := ioutil.ReadFile(path)
  if (err != nil) {
    return EcsToolConfig{}, errors.New("Config file not found at " + path)
  }
  var config EcsToolConfig
  err = json.Unmarshal(data, &config)
  if (err != nil) {
    return EcsToolConfig{}, errors.New("Config file has invalid json")
  }
  fmt.Println(config)
  return config, nil;
}

func handleErr(err error) {
  if aerr, ok := err.(awserr.Error); ok {
    switch aerr.Code() {
    case ecs.ErrCodeServerException:
        fmt.Println(ecs.ErrCodeServerException, aerr.Error())
    case ecs.ErrCodeClientException:
        fmt.Println(ecs.ErrCodeClientException, aerr.Error())
    case ecs.ErrCodeInvalidParameterException:
        fmt.Println(ecs.ErrCodeInvalidParameterException, aerr.Error())
    default:
        fmt.Println(aerr.Error())
    }
  } else {
    // Print the error, cast err to awserr.Error to get the Code and
    // Message from an error.
    fmt.Println(err.Error())
  }
}

func main() {
  path, _ := filepath.Abs("config.json")
  config, err := readConfig(path)
  if (err != nil) {
    panic(err)
  }

  sess := session.Must(session.NewSession(&aws.Config{
      Region: aws.String(endpoints.UsWest2RegionID),
  }))

  svc := ecs.New(sess)
  input := &ecs.ListClustersInput{}

  result, err := svc.ListClusters(input)
  if err != nil {
      handleErr(err)
      return
  }

  clusterArn := *result.ClusterArns[0]
  // fmt.Println(clusterArn)

  // input2 := &ecs.ListServicesInput{Cluster: &clusterArn}
  // result2, err := svc.ListServices(input2)

  // serviceName := "dstld-blue-STAGING"
  serviceNameErr := errors.New("You must pass the service name as the first argument.")

  if (len(os.Args) < 2) {
    handleErr(serviceNameErr)
    return
  }
  serviceName := os.Args[1]
  if (len(serviceName) == 0) {
    handleErr(serviceNameErr)
    return
  }

  // var serviceArn string
  // for a := range result2.ServiceArns {
  //   arn := *result2.ServiceArns[a]
  //   if strings.Contains(arn, serviceName) {
  //     serviceArn = arn
  //   }
  // }

  // fmt.Println(result2)
  // fmt.Println(serviceArn)

  input3 := &ecs.ListTasksInput{Cluster: &clusterArn, ServiceName: &serviceName}
  result3, err := svc.ListTasks(input3)
  if err != nil {
      handleErr(err)
      return
  }
  taskArn := *result3.TaskArns[0]
  // fmt.Println(taskArn)

  input4 := &ecs.DescribeTasksInput{
    Cluster: &clusterArn,
    Tasks: []*string{
        &taskArn,
    },
  }
  result4, err := svc.DescribeTasks(input4)
  if err != nil {
    handleErr(err)
    return
  }
  instanceArn := *result4.Tasks[0].ContainerInstanceArn
  containerName := *result4.Tasks[0].Containers[0].Name
  // fmt.Println(result4)

  input5 := &ecs.DescribeContainerInstancesInput{
    Cluster: &clusterArn,
    ContainerInstances: []*string{
        &instanceArn,
    },
  }
  result5, err := svc.DescribeContainerInstances(input5)

  instanceId := *result5.ContainerInstances[0].Ec2InstanceId

  svc2 := ec2.New(sess)
  input6 := &ec2.DescribeInstancesInput{InstanceIds: []*string{&instanceId}}
  result6, err := svc2.DescribeInstances(input6)
  if err != nil {
    handleErr(err)
    return
  }

  publicIp := *result6.Reservations[0].Instances[0].PublicIpAddress

  // Docker Web API
  resp, err := http.Get(strings.Join([]string{"http://", publicIp, ":51678/v1/tasks"},""))
  if err != nil {
    handleErr(err)
    return
  }
  defer resp.Body.Close()
  body, err := ioutil.ReadAll(resp.Body)

  type TasksResponse struct {
    Tasks []struct{
      Containers []struct {
        Name string
        DockerId string
      }
    }
  }

  var taskIntrospectionMap TasksResponse
  err = json.Unmarshal(body, &taskIntrospectionMap)
  if err != nil {
    handleErr(err)
    return
  }

  // fmt.Println(containerName)
  // fmt.Println(taskIntrospectionMap)


  var dockerId string
  for _, v := range taskIntrospectionMap.Tasks {
    container := v.Containers[0]
    if container.Name == containerName {
      dockerId = container.DockerId
    }
  }

  fmt.Printf("IP: %s\nDocker Id: %s\n", publicIp, dockerId)

  var cmd *exec.Cmd
  if (config.AlwaysEnterContainer) {
    cmd = exec.Command("ssh", "-tt",
      fmt.Sprintf("ec2-user@%s", publicIp),
      fmt.Sprintf("-i %s", config.KeyPath),
      "docker exec -it "+dockerId+" /bin/bash ")
  } else {
    cmd = exec.Command("ssh",
      fmt.Sprintf("ec2-user@%s", publicIp),
      fmt.Sprintf("-i %s", config.KeyPath))
  }
  // cmd.Env = append(cmd.Env, "TERM=xterm")
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr
  cmd.Stdin = os.Stdin // Pseudo-terminal will not be allocated because stdin is not a terminal.

  fmt.Println(fmt.Sprintf("Executing ec2-user@%s -i %s ...", publicIp, config.KeyPath))
  err = cmd.Run()
  if err != nil {
    log.Fatal(err)
  }

  // cmd2 := exec.Command("source", "/etc/default/app")

  stdin, err := cmd.StdinPipe()
  if err != nil {
    log.Fatal(err)
  }

  go func() {
    defer stdin.Close()
    io.WriteString(stdin, "values written to stdin are passed to cmd's standard input")
  }()

  // 2018/12/20 17:25:07 exec: Stdin already set

}


