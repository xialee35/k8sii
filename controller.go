package main

import (
	"fmt"
	v1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8sii/glog"
	appClientset "k8sii/pkg/client/clientset/versioned"
	appinformer "k8sii/pkg/client/informers/externalversions/controllerexample/v1alpha1"
	appListers "k8sii/pkg/client/listers/controllerexample/v1alpha1"
	//v1 "k8s.io/client-go/listers/core/v1"
	//appslisters "k8s.io/client-go/listers/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	controllerexamplev1alpha1 "k8sii/pkg/apis/controllerexample/v1alpha1"
	appscheme "k8sii/pkg/client/clientset/versioned/scheme"
)

const controllerAgentName = "app-controller"

type Controller struct {
	kubeclientset kubernetes.Interface
	appclientset  appClientset.Interface
	// deploymentsLister appslisters.DeploymentLister
	appsListers appListers.AppLister
	appsSynced  cache.InformerSynced
	workqueue   workqueue.RateLimitingInterface
	recorder    record.EventRecorder
}

func NewController(kubeclientset kubernetes.Interface, appclientset appClientset.Interface, appInformer appinformer.AppInformer) *Controller {
	//
	utilruntime.Must(appscheme.AddToScheme(scheme.Scheme))
	glog.V(4).Info("creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(glog.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset,
		appclientset,
		appInformer.Lister(),
		appInformer.Informer().HasSynced,
		workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "App"),
		recorder,
	}
	glog.Info("setting event handlers")

	appInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueApp,
		UpdateFunc: func(old, new interface{}) {
			oldApp := old.(*controllerexamplev1alpha1.App)
			newApp := new.(*controllerexamplev1alpha1.App)
			if oldApp.ResourceVersion == newApp.ResourceVersion {
				glog.Info("版本一致，表示没有实际的更新操作，立即返回")
				return
			}
			controller.enqueueApp(new)
		},
		DeleteFunc: controller.enqueueAppForDelete,
	})
	return controller

}

/**
数据先发放入缓存，再入队列
*/
func (c *Controller) enqueueApp(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}

func (c *Controller) enqueueAppForDelete(obj interface{}) {
	var key string
	var err error
	//从缓存中删除指定对象
	key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)

	if err != nil {
		glog.Info("删除缓存中的对象失败，错误信息", err.Error())
		utilruntime.HandleError(err)
		return
	}
	//在将key放入队列

	c.workqueue.AddRateLimited(key)
}

//处理
func (c *Controller) syncHandler(key string) error {
	//convert the namespace/name string into a distinct namespace and name

	namespaces, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		glog.Error(err.Error())
		return err
	}

	//从缓存中取对象
	app, err := c.appsListers.Apps(namespaces).Get(name)
	if err != nil {
		// 如果APP对象被删除了，就会走到这里，所以应该在这里加入执行
		if errors.IsNotFound(err) {
			glog.Infof("App被删掉了，请在这里执行实际业务逻辑%s，%s ...", namespaces, name)
			return nil
		}
		glog.Error(err.Error())
		return err
	}
	c.createDeployment(namespaces, name)
	glog.Infof("这里是app对象的期望状态：%#v", app)
	glog.Infof("实际状态是从业务层面得到的，此处应该去的实际状态，与期望状态做对比，并根据差异做出响应(新增或者删除)")

	return nil
}

func (c *Controller) createDeployment(namespaces string, name string) {
	//namespaces,name,err := cache.SplitMetaNamespaceKey(key)
	labels := make(map[string]string)
	labels["name"] = name
	var num *int32
	var i int32 = 2
	num = &i
	deployment := v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myapps",
			Namespace: namespaces,
			Labels:    labels,
		},
		Spec: v1.DeploymentSpec{
			Replicas: num,
			Template: corev1.PodTemplateSpec{
				metav1.ObjectMeta{
					Name:      "myapps",
					Namespace: namespaces,
					Labels:    labels,
				},
				corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  name,
							Image: "tomcat",
						},
					},
				},
			},
		},
	}
	dep, err := c.kubeclientset.AppsV1().Deployments(namespaces).Create(&deployment)
	if err != nil {
		fmt.Println("发布失败，", err.Error())
	}

	fmt.Println("发布成功，", dep)
}

//在此处开始controller的业务
func (c *Controller) run(threadiness int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	fmt.Println("开始controller的业务，开始一次缓存数据同步")
	glog.Info("开始controller业务，开始一次缓存数据同步")
	if ok := cache.WaitForCacheSync(stopCh, c.appsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	glog.Info("worker启动")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	glog.Info("worker已经启动")
	<-stopCh
	glog.Info("worker已经结束")

	return nil
}

func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// 取数据处理
func (c *Controller) processNextWorkItem() bool {

	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)
		var key string
		var ok bool

		if key, ok = obj.(string); !ok {

			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// 在syncHandler中处理业务
		if err := c.syncHandler(key); err != nil {
			return fmt.Errorf("error syncing '%s': %s", key, err.Error())
		}

		c.workqueue.Forget(obj)
		glog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}
