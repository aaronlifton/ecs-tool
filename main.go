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
  "time"
)

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

  serviceName := "dstld-blue-STAGING"
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
  // fmt.Println(publicIp)


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

  certPath := "/Users/aaron/Downloads/consul_key.pem" // need to get absolute path from rel?
  cmd := exec.Command("ssh", fmt.Sprintf("ec2-user@%s", publicIp), fmt.Sprintf("-i %s", certPath))
  // cmd.Env = append(cmd.Env, "TERM=xterm")
  cmd.Stdout = os.Stdout
  cmd.Stderr = os.Stderr
  cmd.Stdin = os.Stdin // Pseudo-terminal will not be allocated because stdin is not a terminal.

  // r,w,_ := os.Pipe()
  // os.Stdin = r
  // w.WriteString(fmt.Sprintf("docker exec -it %s /bin/bash", dockerId))
  go func() {
    time.Sleep(11 * time.Second)
    os.Stdin.Write([]byte("Hello\n\r"))
  }()

  // pipe
  // w.Close()
  // end pipe

  fmt.Println(fmt.Sprintf("Executing ec2-user@%s -i %s ...", publicIp, certPath))
  err = cmd.Run()
  if err != nil {
    log.Fatal(err)
  }

}


