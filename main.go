/*
Copyright 2016 Juniper

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

*/
package main

import (
	"math/rand"
        "time"
	"fmt"
	"net"
	"strings"
        "io/ioutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
        apiv1 "k8s.io/api/core/v1"
        //appsv1 "k8s.io/api/apps/v1"
        "gopkg.in/yaml.v2"
)


func main(){
   label := os.Args[1]
   err := createConfig(label)
   if err != nil {
     panic(err.Error())
   }
}

func createConfig(label string) error{
        return retry(1, time.Second, func() error {
	  labelString := fmt.Sprintf("app=%s",label)
          myNodeName := os.Getenv("MY_NODE_NAME")
          nameSpaceByte, err := ioutil.ReadFile("/run/secrets/kubernetes.io/serviceaccount/namespace")
          if err != nil {
                panic(err.Error())
          }
          nameSpace := string(nameSpaceByte)
	  config, err := rest.InClusterConfig()
	  if err != nil {
		panic(err.Error())
	  }
	  clientset, err := kubernetes.NewForConfig(config)
	  if err != nil {
		panic(err.Error())
	  }
          replicaSetClient := clientset.AppsV1().ReplicaSets(nameSpace)
	  list, err := replicaSetClient.List(metav1.ListOptions{LabelSelector: labelString,})
	  if err != nil {
		panic(err)
	   }
	  replicas := list.Items[0].Spec.Replicas
	  fmt.Println(*replicas)

          kubeadmConfigMapClient := clientset.CoreV1().ConfigMaps("kube-system")
          kcm, err := kubeadmConfigMapClient.Get("kubeadm-config", metav1.GetOptions{})
          clusterConfig := kcm.Data["ClusterConfiguration"]
          clusterConfigByte := []byte(clusterConfig)
          clusterConfigMap := make(map[interface{}]interface{})
          err = yaml.Unmarshal(clusterConfigByte, &clusterConfigMap)
          if err != nil {
            return err
          }
          controlPlaneEndpoint := clusterConfigMap["controlPlaneEndpoint"].(string)
          controlPlaneEndpointHost, controlPlaneEndpointPort, _ := net.SplitHostPort(controlPlaneEndpoint)
          clusterName := clusterConfigMap["clusterName"].(string)

          networkConfig := make(map[interface{}]interface{})
          networkConfig = clusterConfigMap["networking"].(map[interface{}]interface{})
          podSubnet := networkConfig["podSubnet"].(string)
          serviceSubnet := networkConfig["serviceSubnet"].(string)

          configMap := &apiv1.ConfigMap{
              ObjectMeta: metav1.ObjectMeta{
                  Name: "contrailcontrollernodesv1",
                  Namespace: nameSpace,
              },
              Data: map[string]string{
                  "CONTROLLER_NODES": myNodeName,
                  "KUBERNETES_API_SERVER": controlPlaneEndpointHost,
                  "KUBERNETES_API_SECURE_PORT": controlPlaneEndpointPort,
                  "KUBERNETES_POD_SUBNETS": podSubnet,
                  "KUBERNETES_SERVICE_SUBNETS": serviceSubnet,
                  "KUBERNETES_CLUSTER_NAME": clusterName,
              },
          }

          configMapClient := clientset.CoreV1().ConfigMaps(nameSpace)
          cm, err := configMapClient.Get("contrailcontrollernodesv1", metav1.GetOptions{})
	  var numOfControllers int = 1
	  numOfReplicas := int(*replicas)
          if err != nil {
            configMapClient.Create(configMap)
            fmt.Printf("Created %s\n", cm.Name)
          } else {
            controllerNodes := cm.Data["CONTROLLER_NODES"]
            controllerNodesList := strings.Split(controllerNodes,",")
	    numOfControllers = len(controllerNodesList)
          }
	  for numOfControllers < numOfReplicas {
		r := rand.Intn(4)
		time.Sleep(time.Duration(r) * time.Second)
	        //time.Sleep(2 * time.Second)
		cm, err := configMapClient.Get("contrailcontrollernodesv1", metav1.GetOptions{})
		if err != nil {
			panic(err)
		}
		controllerNodes := cm.Data["CONTROLLER_NODES"]	
		controllerNodesList := strings.Split(controllerNodes,",")
		inList := false
                for _, controllerNode := range controllerNodesList {
			if controllerNode == myNodeName {
				inList = true
			}
		}
		if inList == false {
			controllerNodesList = append(controllerNodesList, myNodeName)
		}
		newControllerString := strings.Join(controllerNodesList,",")
		numOfControllers = len(controllerNodesList)
                configMap := &apiv1.ConfigMap{
               		ObjectMeta: metav1.ObjectMeta{
                      	  Name: "contrailcontrollernodesv1",
                          Namespace: nameSpace,
                        },
                        Data: map[string]string{
                                "CONTROLLER_NODES": newControllerString,
                                "KUBERNETES_API_SERVER": controlPlaneEndpointHost,
                                "KUBERNETES_API_SECURE_PORT": controlPlaneEndpointPort,
                                "KUBERNETES_POD_SUBNETS": podSubnet,
                                "KUBERNETES_SERVICE_SUBNETS": serviceSubnet,
                                "KUBERNETES_CLUSTER_NAME": clusterName,
                       },
                }
		configMapClient.Update(configMap)
          	fmt.Println("numOfControllers", numOfControllers)
          	fmt.Println("numOfReplicas", numOfReplicas)
	  }
          return nil
        })
}
