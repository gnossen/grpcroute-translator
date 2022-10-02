package main

import (
	"grpcroutetranslator/grpcroutetranslation"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/gateway-api/apis/v1alpha2"
	"sigs.k8s.io/gateway-api/apis/v1beta1"
	"sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"

	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"time"
)

var local = flag.Bool("local", false, "Indicates that the server is not running in a cluster.")

func getHttpRouteNameForGrpcRouteName(grname string) string {
	return fmt.Sprintf("grpcroute-translator-%s", grname)
}

func getHttpRouteUri(hrname, namespace string) string {
	return fmt.Sprintf("/apis/gateway.networking.k8s.io/v1beta1/namespaces/%s/httproutes/%s", namespace, hrname)
}

func getHttpRouteForGrpcRoute(gr *v1alpha2.GRPCRoute) (hr *v1beta1.HTTPRoute, uri string, err error) {
	hrpspec, err := grpcroutetranslation.TranslateGRPCRoute(gr.Spec)
	if err != nil {
		err = fmt.Errorf("Failed to translate: %s", err)
		return
	}
	hrname := getHttpRouteNameForGrpcRouteName(gr.Name)
	uri = getHttpRouteUri(hrname, gr.Namespace)
	hr = &v1beta1.HTTPRoute{
		Spec: hrpspec,
		TypeMeta: metav1.TypeMeta{
			Kind:       "HTTPRoute",
			APIVersion: "gateway.networking.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hrname,
			Namespace: gr.Namespace,
		},
	}
	return
}

func updateOnDeltas(clientset *versioned.Clientset) {
	watchlist := cache.NewListWatchFromClient(
		clientset.GatewayV1alpha2().RESTClient(),
		"grpcroutes",
		corev1.NamespaceAll,
		fields.Everything())

	_, controller := cache.NewInformer(
		watchlist,
		&v1alpha2.GRPCRoute{},
		time.Second*5, // TODO: Make this configurable.
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				gr := *obj.(*v1alpha2.GRPCRoute)
				fmt.Printf("Adding for GRPCRoute %s\n", gr.Name)
				hr, uri, err := getHttpRouteForGrpcRoute(&gr)
				if err != nil {
					fmt.Printf("%s", err)
					return
				}
				body, err := json.Marshal(hr)
				if err != nil {
					fmt.Printf("Failed to marshal HTTPRoute: %s", err)
					return
				}

				// TODO; Something better than context.TODO.
				resp, err := clientset.RESTClient().
					Post().
					AbsPath(uri).
					Body(body).
					DoRaw(context.TODO())
				if err != nil {
					fmt.Printf("Failed to create HTTPRoute: %s\n  Response: %s", err, string(resp[:]))
				}
			},
			DeleteFunc: func(obj interface{}) {
				gr := *obj.(*v1alpha2.GRPCRoute)
				fmt.Printf("Deleting for GRPCRoute %s\n", gr.Name)
				uri := getHttpRouteUri(getHttpRouteNameForGrpcRouteName(gr.Name), gr.Namespace)
				resp, err := clientset.RESTClient().
					Delete().
					AbsPath(uri).
					DoRaw(context.TODO())
				if err != nil {
					fmt.Printf("Failed to delete HTTPRoute: %s\n  Response: %s", err, string(resp[:]))
				}
			},
			UpdateFunc: func(oldobj, newobj interface{}) {
				// Probably just treat this like an Add as long as that operation is idempotent.
				newgr := *newobj.(*v1alpha2.GRPCRoute)
				fmt.Printf("Updating for GRPCRoute %s\n", newgr.Name)
				hr, uri, err := getHttpRouteForGrpcRoute(&newgr)
				if err != nil {
					fmt.Printf("%s", err)
					return
				}
				body, err := json.Marshal(hr)
				if err != nil {
					fmt.Printf("Failed to marshal HTTPRoute: %s", err)
					return
				}

				// TODO: Write an idempotent version of this.
				// TODO; Something better than context.TODO.
				resp, err := clientset.RESTClient().
					Post().
					AbsPath(uri).
					Body(body).
					DoRaw(context.TODO())
				if err != nil {
					fmt.Printf("Failed to update HTTPRoute: %s\n  Response: %s", err, string(resp[:]))
				}
			},
		})

	controller.Run(make(chan struct{}))
}

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	var config *rest.Config
	var err error
	if *local {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		panic(err.Error())
	}

	// clientset, err := kubernetes.NewForConfig(config)
	clientset, err := versioned.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	updateOnDeltas(clientset)
}
