package kubeclient

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func New() (dynamic.Interface, error) {
	config, err := config()
	if err != nil {
		return nil, err
	}

	return dynamic.NewForConfig(config)
}

func ObjectFromAdmissionRequest(client dynamic.Interface, req *v1beta1.AdmissionRequest) (metav1.Object, error) {
	identifier := schema.GroupVersionResource{
		Group:    req.Resource.Group,
		Version:  req.Resource.Version,
		Resource: req.Resource.Resource,
	}
	c := client.Resource(identifier)
	return c.Namespace(req.Namespace).Get(req.Name, metav1.GetOptions{})
}

func kubeconfig() (string, error) {
	env, found := os.LookupEnv("KUBECONFIG")
	if !found {
		return "", fmt.Errorf("KUBECONFIG environment variable not found")
	}
	return env, nil
}

func config() (*rest.Config, error) {
	path, err := kubeconfig()
	if err != nil {
		log.Info(err.Error())
		log.Info("assuming running inside Kubernetes, using in-cluster configuration")
		return rest.InClusterConfig()
	} else {
		log.Infof("using configuration from '%s'", path)
		return clientcmd.BuildConfigFromFlags("", path)
	}
}
