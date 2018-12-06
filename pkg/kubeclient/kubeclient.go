package kubeclient

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func New() (*kubernetes.Clientset, error) {
	config, err := config()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
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
