/*
Copyright 2016 Juniper

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

*/
package main

import (
	"sort"
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
	"gopkg.in/yaml.v2"
)


func main(){
        if len(os.Args) != 2 {
		panic("wrong number of args")
	}
	label := os.Args[1]
	err := createConfig(label)
	if err != nil {
		panic(err.Error())
	}
}

func createConfig(label string) error{
	return retry(1, time.Second, func() error {
		labelString := fmt.Sprintf("app=%s",label)
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
		podTemplateHash := list.Items[0].Labels["pod-template-hash"]
		replicas := list.Items[0].Spec.Replicas
		numOfReplicas := int(*replicas)
		kubeadmConfigMapClient := clientset.CoreV1().ConfigMaps("kube-system")
		kcm, err := kubeadmConfigMapClient.Get("kubeadm-config", metav1.GetOptions{})
		clusterConfig := kcm.Data["ClusterConfiguration"]
		clusterConfigByte := []byte(clusterConfig)
		clusterConfigMap := make(map[interface{}]interface{})
		err = yaml.Unmarshal(clusterConfigByte, &clusterConfigMap)
		if err != nil {
			panic(err)
		}
		controlPlaneEndpoint := clusterConfigMap["controlPlaneEndpoint"].(string)
		controlPlaneEndpointHost, controlPlaneEndpointPort, _ := net.SplitHostPort(controlPlaneEndpoint)
		clusterName := clusterConfigMap["clusterName"].(string)
		networkConfig := make(map[interface{}]interface{})
		networkConfig = clusterConfigMap["networking"].(map[interface{}]interface{})
		podSubnet := networkConfig["podSubnet"].(string)
		serviceSubnet := networkConfig["serviceSubnet"].(string)
		var controllerPodList []string
		var nodeListString string
		numOfPods := len(controllerPodList)
		for numOfPods < numOfReplicas {
			podClient := clientset.CoreV1().Pods(nameSpace)
			podList, err := podClient.List(metav1.ListOptions{})
			if err != nil {
				panic(err)
			}
			for _,pod := range podList.Items {
				if strings.Contains(pod.Name, podTemplateHash){
					controllerPodList = append(controllerPodList,pod.Name)
				}
			}
			sortableControllerPodList := sort.StringSlice(controllerPodList)
			sort.Sort(sortableControllerPodList)
			numOfPods = len(controllerPodList)
			if numOfPods == numOfReplicas {
				if os.Getenv("MY_POD_NAME") == sortableControllerPodList[0] {
					var nodeList []string
					for _,pod := range sortableControllerPodList {
						currentPod, err := podClient.Get(pod, metav1.GetOptions{})
						if err != nil {
							panic(err)
						}
						nodeList = append(nodeList, currentPod.Spec.NodeName)
						nodeListString = strings.Join(nodeList,",")
					}
				}
			}
		}
		if nodeListString != "" {
			configMap := &apiv1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "contrailcontrollernodesv1",
					Namespace: nameSpace,
					},
				Data: map[string]string{
					"CONTROLLER_NODES": nodeListString,
					"KUBERNETES_API_SERVER": controlPlaneEndpointHost,
					"KUBERNETES_API_SECURE_PORT": controlPlaneEndpointPort,
					"KUBERNETES_POD_SUBNETS": podSubnet,
					"KUBERNETES_SERVICE_SUBNETS": serviceSubnet,
					"KUBERNETES_CLUSTER_NAME": clusterName,
					},
			}
			configMapClient := clientset.CoreV1().ConfigMaps(nameSpace)
			cm, err := configMapClient.Get("contrailcontrollernodesv1", metav1.GetOptions{})
			if err != nil {
				configMapClient.Create(configMap)
			        fmt.Println("created ", cm.Name)
			}
		}
		return nil
	})
}
