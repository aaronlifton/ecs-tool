package awsutil
import (
  // "github.com/aws/aws-sdk-go/aws"
  "fmt"
  "github.com/aws/aws-sdk-go/service/ecs"
  "strings"
)

func GetServiceArn(ecsInstance *ecs.ECS, clusterArn string, serviceName string) string {
  input2 := ecs.ListServicesInput{Cluster: &clusterArn}
  result2, err := ecsInstance.ListServices(&input2)
  if (err != nil) {
    fmt.Println(err)
    return ""
  }
  var serviceArn string
  for a := range result2.ServiceArns {
    arn := *result2.ServiceArns[a]
    if strings.Contains(arn, serviceName) {
      serviceArn = arn
    }
  }
  return serviceArn
}