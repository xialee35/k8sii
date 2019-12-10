package main

import (
	"flag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8sii/glog"
	appClientset "k8sii/pkg/client/clientset/versioned"
	appinformer "k8sii/pkg/client/informers/externalversions"
	"k8sii/pkg/signals"
	"time"
)

var (
	masterURL  string
	kubeconfig string
)

func main() {
	flag.Parse()
	// 处理信号量
	stopCh := signals.SetupSignalHandler()
	cfg, err := clientcmd.BuildConfigFromFlags("", "")
	if err != nil {
		glog.Fatalf("Error building kubeconfig: %s", err.Error())
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}
	appsClient, err := appClientset.NewForConfig(cfg)
	if err != nil {
		glog.Fatalf("Error building example clientset: %s", err.Error())
	}

	appInformerFactory := appinformer.NewSharedInformerFactory(appsClient, time.Second*30)

	controller := NewController(kubeClient, appsClient, appInformerFactory.Controllerexample().V1alpha1().Apps())

	//启动informer
	go appInformerFactory.Start(stopCh)

	//controller开始处理消息
	if err = controller.run(2, stopCh); err != nil {
		glog.Fatalf("Error running controller: %s", err.Error())
	}

}

func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
}
