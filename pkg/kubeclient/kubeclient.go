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

func New(config *rest.Config) (dynamic.Interface, error) {
	return dynamic.NewForConfig(config)
}

func namespacedObject(client dynamic.Interface, req v1beta1.AdmissionRequest, identifier schema.GroupVersionResource) (metav1.Object, error) {
	log.Debugf("using %+v to look up resource '%s' in namespace '%s'", identifier, req.Name, req.Namespace)
	c := client.Resource(identifier)
	return c.Namespace(req.Namespace).Get(req.Name, metav1.GetOptions{})
}

func clusterObject(client dynamic.Interface, req v1beta1.AdmissionRequest, identifier schema.GroupVersionResource) (metav1.Object, error) {
	log.Debugf("using %+v to look up resource '%s' in cluster scope", identifier, req.Name)
	c := client.Resource(identifier)
	return c.Get(req.Name, metav1.GetOptions{})
}

func ObjectFromAdmissionRequest(client dynamic.Interface, req v1beta1.AdmissionRequest) (metav1.Object, error) {
	if len(req.Name) == 0 {
		return nil, fmt.Errorf("resource name must be specified")
	}
	identifier := schema.GroupVersionResource{
		Group:    req.Resource.Group,
		Version:  req.Resource.Version,
		Resource: req.Resource.Resource,
	}
	if len(req.Namespace) == 0 {
		return clusterObject(client, req, identifier)
	}
	return namespacedObject(client, req, identifier)
}

func kubeconfig() (string, error) {
	env, found := os.LookupEnv("KUBECONFIG")
	if !found {
		return "", fmt.Errorf("KUBECONFIG environment variable not found")
	}
	return env, nil
}

func Config() (*rest.Config, error) {
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
