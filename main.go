package main

import (
	"flag"
	"fmt"
	"github.com/golang/glog"
	pipeline "github.com/rawlingsj/pipeline-controller/apis/pipeline/v1"
	pipelineclient "github.com/rawlingsj/pipeline-controller/client"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"
	"log"
	"time"

	// Uncomment the following line to load the gcp plugin (only required to authenticate against GKE clusters).
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

type Controller struct {
	indexer  cache.Indexer
	queue    workqueue.RateLimitingInterface
	informer cache.Controller
}

func NewController(queue workqueue.RateLimitingInterface, indexer cache.Indexer, informer cache.Controller) *Controller {
	return &Controller{
		informer: informer,
		indexer:  indexer,
		queue:    queue,
	}
}

func (c *Controller) processNextItem() bool {
	// Wait until there is a new item in the working queue
	key, quit := c.queue.Get()
	if quit {
		return false
	}
	// Tell the queue that we are done with processing this key. This unblocks the key for other workers
	// This allows safe parallel processing because two pods with the same key are never processed in
	// parallel.
	defer c.queue.Done(key)

	// Invoke the method containing the business logic
	err := c.syncToStdout(key.(string))
	// Handle the error if something went wrong during the execution of the business logic
	c.handleErr(err, key)
	return true
}

// syncToStdout is the business logic of the controller. In this controller it simply prints
// information about the pod to stdout. In case an error happened, it has to simply return the error.
// The retry logic should not be part of the business logic.
func (c *Controller) syncToStdout(key string) error {
	obj, exists, err := c.indexer.GetByKey(key)
	if err != nil {
		glog.Errorf("Fetching object with key %s from store failed with %v", key, err)
		return err
	}

	if !exists {
		// Below we will warm up our cache with a Pipeline, so that we will see a delete for one pod
		fmt.Printf("Pipeline %s does not exist anymore\n", key)
	} else {
		// Note that you also have to check the uid if you have a local controlled resource, which
		// is dependent on the actual instance, to detect that a Pod was recreated with the same name
		fmt.Printf("Sync/Add/Update for Pipeline %s\n", obj.(*pipeline.Pipeline).GetName())
	}
	return nil
}

// handleErr checks if an error happened and makes sure we will retry later.
func (c *Controller) handleErr(err error, key interface{}) {
	if err == nil {
		// Forget about the #AddRateLimited history of the key on every successful synchronization.
		// This ensures that future processing of updates for this key is not delayed because of
		// an outdated error history.
		c.queue.Forget(key)
		return
	}

	// This controller retries 5 times if something goes wrong. After that, it stops trying.
	if c.queue.NumRequeues(key) < 5 {
		glog.Infof("Error syncing Pipeline %v: %v", key, err)

		// Re-enqueue the key rate limited. Based on the rate limiter on the
		// queue and the re-enqueue history, the key will be processed later again.
		c.queue.AddRateLimited(key)
		return
	}

	c.queue.Forget(key)
	// Report to an external entity that, even after several retries, we could not successfully process this key
	runtime.HandleError(err)
	glog.Infof("Dropping Pipeline %q out of the queue: %v", key, err)
}

func (c *Controller) Run(threadiness int, stopCh chan struct{}) {
	defer runtime.HandleCrash()

	// Let the workers stop when we are done
	defer c.queue.ShutDown()
	glog.Info("Starting Pipeline controller")

	go c.informer.Run(stopCh)

	// Wait for all involved caches to be synced, before processing items from the queue is started
	if !cache.WaitForCacheSync(stopCh, c.informer.HasSynced) {
		runtime.HandleError(fmt.Errorf("Timed out waiting for caches to sync"))
		return
	}

	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	<-stopCh
	glog.Info("Stopping Pipeline controller")
}

func (c *Controller) runWorker() {
	for c.processNextItem() {
	}
}

func main() {
	var kubeconfig string
	var master string

	flag.StringVar(&kubeconfig, "kubeconfig", "", "absolute path to the kubeconfig file")
	flag.StringVar(&master, "master", "", "master url")
	flag.Parse()
	flag.Set("logtostderr", "true")

	// Create the client config. Use masterURL and kubeconfig if given, otherwise assume in-cluster.
	config, err := clientcmd.BuildConfigFromFlags(master, kubeconfig)
	if err != nil {
		panic(err)
	}

	// make a new config for our extension's API group, using the first config as a baseline
	client, _, err := pipelineclient.NewClient(config)
	if err != nil {
		panic(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	// create the pod watcher
	pipelineListWatcher := cache.NewListWatchFromClient(client, "pipelines", v1.NamespaceDefault, fields.Everything())

	// create the workqueue
	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	indexer, informer := cache.NewIndexerInformer(pipelineListWatcher, &pipeline.Pipeline{}, 0, cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			log.Println("Pipeline job created")
			deployOneShotMaster(clientset)
		},
		UpdateFunc: func(old interface{}, new interface{}) {
			log.Println("Pipeline job updated")
		},
		DeleteFunc: func(obj interface{}) {
			log.Println("Pipeline job deleted")
		},
	}, cache.Indexers{})

	controller := NewController(queue, indexer, informer)

	// Now let's start the controller
	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(1, stop)

	// Wait forever
	select {}
}

func deployOneShotMaster(client *kubernetes.Clientset) {
	log.Println("TODO: deploy oneshot master")

	//jobClient := client.BatchV1().Jobs(v1.NamespaceDefault)
	//_, err := jobClient.Get("jenkins", meta_v1.GetOptions{})
	//if err != nil {
	//
	//	//resp, err := http.Get("https://gist.githubusercontent.com/rawlingsj/707185dfa5aeaf828551b18de827f494/raw/b3d3dfd1e2e48fe34141c28ec625d2ed87fb8af9/gistfile1.txt")
	//	//if err != nil {
	//	//	panic(err)
	//	//}
	//	//defer resp.Body.Close()
	//	//
	//	//body, err := ioutil.ReadAll(resp.Body)
	//	//if err != nil {
	//	//	panic(err)
	//	//}
	//	//
	//	//rs := client.BatchV1().RESTClient().Post().Body(body).Do()
	//
	//	e := exec.Command("kubectl", "apply", "-f", "https://gist.githubusercontent.com/rawlingsj/707185dfa5aeaf828551b18de827f494/raw/b3d3dfd1e2e48fe34141c28ec625d2ed87fb8af9/gistfile1.txt")
	//	err := e.Run()
	//	if err != nil {
	//		panic(err)
	//	}
	//}
}
