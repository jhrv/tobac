package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/nais/tobac/pkg/kubeclient"
	"github.com/nais/tobac/pkg/metrics"
	"github.com/nais/tobac/pkg/teams"
	"github.com/nais/tobac/pkg/tobac"
	"github.com/nais/tobac/pkg/version"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile             string
	KeyFile              string
	LogFormat            string
	AzureTimeout         string
	AzureSyncInterval    string
	ServiceUserTemplates []string
	ClusterAdmins        []string
	LogLevel             string
	APIServerInsecureTLS bool
}

func DefaultConfig() *Config {
	return &Config{
		CertFile:             "/etc/tobac/tls.crt",
		KeyFile:              "/etc/tobac/tls.key",
		AzureTimeout:         "5s",
		AzureSyncInterval:    "10m",
		ServiceUserTemplates: []string{"system:serviceaccount:default:serviceuser-%s"},
		LogFormat:            "text",
		LogLevel:             "info",
		APIServerInsecureTLS: false,
	}
}

var config = DefaultConfig()

var kubeClient dynamic.Interface

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "cert", c.CertFile, "File containing the x509 certificate for HTTPS.")
	flag.StringVar(&c.KeyFile, "key", c.KeyFile, "File containing the x509 private key.")
	flag.StringVar(&c.LogFormat, "log-format", c.LogFormat, "Log format, either 'json' or 'text'.")
	flag.StringVar(&c.AzureSyncInterval, "azure-sync-interval", c.AzureSyncInterval, "How often to synchronize the team list against Azure AD.")
	flag.StringVar(&c.AzureTimeout, "azure-timeout", c.AzureSyncInterval, "Query timeout during Azure AD synchronization.")
	flag.StringSliceVar(&c.ServiceUserTemplates, "service-user-templates", c.ServiceUserTemplates, "List of Kubernetes users that will be granted access to resources. %s will be replaced by the team label.")
	flag.StringSliceVar(&c.ClusterAdmins, "cluster-admins", c.ClusterAdmins, "Commas-separated list of groups that are allowed to perform any action.")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "Logging verbosity level.")
	flag.BoolVar(&c.APIServerInsecureTLS, "apiserver-insecure-tls", c.APIServerInsecureTLS, "Turn off TLS verification for the Kubernetes API server connection.")
}

func genericErrorResponse(format string, a ...interface{}) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Allowed: false,
		Result: &metav1.Status{
			Message: fmt.Sprintf(format, a...),
		},
	}
}

func decode(raw []byte) (*tobac.KubernetesResource, error) {
	k := &tobac.KubernetesResource{}
	if len(raw) == 0 {
		return nil, nil
	}

	r := bytes.NewReader(raw)
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(k); err != nil {
		return nil, fmt.Errorf("while decoding Kubernetes resource: %s", err)
	}

	return k, nil
}

func admitCallback(ar v1beta1.AdmissionReview) (*v1beta1.AdmissionResponse, error) {
	if ar.Request == nil {
		return nil, fmt.Errorf("admission review request is empty")
	}

	previous, err := decode(ar.Request.OldObject.Raw)
	if err != nil {
		return nil, fmt.Errorf("while decoding old resource: %s", err)
	}

	resource, err := decode(ar.Request.Object.Raw)
	if err != nil {
		return nil, fmt.Errorf("while decoding resource: %s", err)
	}

	req := tobac.Request{
		UserInfo:             ar.Request.UserInfo,
		ExistingResource:     previous,
		SubmittedResource:    resource,
		ClusterAdmins:        config.ClusterAdmins,
		ServiceUserTemplates: config.ServiceUserTemplates,
		TeamProvider:         teams.Get,
	}

	var selfLink string
	if previous != nil {
		selfLink = previous.GetSelfLink()
	} else if resource != nil {
		selfLink = resource.GetSelfLink()
	}

	if len(selfLink) > 0 {
		log.Infof("Request '%s' from user '%s' in groups %+v", selfLink, ar.Request.UserInfo.Username, ar.Request.UserInfo.Groups)
	} else {
		log.Infof("Request from user '%s' in groups %+v", ar.Request.UserInfo.Username, ar.Request.UserInfo.Groups)
	}

	// If this is a request to execute a command in a pod, the original resource is not sent with the request,
	// and we need to retrieve it to check team membership. Thus, we delete the original objects and fetch only
	// the parent resource.
	if ar.Request.Resource.Resource == "pods" && ar.Request.SubResource == "exec" {
		resource = nil
		previous = nil
	}

	// These checks are needed in order to avoid a null pointer exception in tobac.Allowed().
	// Interfaces can be nil checked, but the instances they're pointing to can be nil and
	// still pass through that check.
	if previous == nil {
		req.ExistingResource = nil
	}
	if resource == nil {
		req.SubmittedResource = nil
	}

	// If this is a DELETE request, the previous resource is not included,
	// and we need to retrieve the object from the Kubernetes API server.
	//
	// See https://github.com/kubernetes/kubernetes/pull/27193
	// See https://github.com/kubernetes/kubernetes/pull/66535
	//
	if resource == nil && previous == nil {
		log.Debug("attempting to fetch object from Kubernetes")
		e, err := kubeclient.ObjectFromAdmissionRequest(kubeClient, *ar.Request)
		if err != nil {
			// Cluster administrators know what they're doing [sic] and
			// are immune to failure when objects don't exist.
			if tobac.ClusterAdminResponse(req) == nil {
				return nil, fmt.Errorf("while retrieving resource: %s", err)
			} else {
				log.Debugf("Previous object does not exist; ignoring because requester is cluster administrator")
			}
		} else {
			selfLink = e.GetSelfLink()
			log.Debugf("Previous object retrieved from %s", e.GetSelfLink())
			req.ExistingResource = e
		}
	}

	log.Tracef("parsed/old: %+v", previous)
	log.Tracef("parsed/new: %+v", resource)

	response := tobac.Allowed(req)

	reviewResponse := &v1beta1.AdmissionResponse{
		Allowed: response.Allowed,
		Result: &metav1.Status{
			Message: response.Reason,
		},
	}

	fields := log.Fields{
		"user":        ar.Request.UserInfo.Username,
		"groups":      ar.Request.UserInfo.Groups,
		"namespace":   ar.Request.Namespace,
		"operation":   ar.Request.Operation,
		"subresource": ar.Request.SubResource,
		"resource":    selfLink,
	}
	logEntry := log.WithFields(fields)

	if response.Allowed {
		logEntry.Infof("Request allowed: %s", response.Reason)
	} else {
		logEntry.Warningf("Request denied: %s", response.Reason)
	}

	return reviewResponse, nil
}

func reply(r *http.Request) (*v1beta1.AdmissionReview, error) {
	var err error

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		return nil, fmt.Errorf("contentType=%s, expect application/json", contentType)
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("while reading admission request: %s", err)
	}

	log.Tracef("request: %s", string(data))

	decoder := json.NewDecoder(bytes.NewReader(data))
	err = decoder.Decode(&ar)
	if err == nil {
		reviewResponse, err = admitCallback(ar)
	}

	if err != nil {
		reviewResponse = genericErrorResponse(err.Error())
	}

	reviewResponse.UID = ar.Request.UID

	return &v1beta1.AdmissionReview{
		Response: reviewResponse,
	}, nil
}

func serve(w http.ResponseWriter, r *http.Request) {
	review, err := reply(r)

	if err != nil {
		log.Errorf("while generating review response: %s", err)
	}

	// if there is no review response at this point, we simply cannot provide the API server with a meaningful reply
	// because we couldn't decode a request UID.
	if review == nil {
		return
	}

	if review.Response.Allowed {
		metrics.Admitted.Inc()
	} else {
		metrics.Denied.Inc()
	}

	encoder := json.NewEncoder(w)
	err = encoder.Encode(review)
	if err != nil {
		log.Errorf("while sending review response: %s", err)
	}
}

func configTLS(config Config) (*tls.Config, error) {
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("while loading certificate and key file: %s", err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
	}, nil
}

func textFormatter() log.Formatter {
	return &log.TextFormatter{
		DisableTimestamp: false,
		FullTimestamp:    true,
	}
}

func jsonFormatter() log.Formatter {
	return &log.JSONFormatter{
		TimestampFormat: time.RFC3339Nano,
	}
}

func run() error {
	config.addFlags()
	flag.Parse()

	switch config.LogFormat {
	case "json":
		log.SetFormatter(jsonFormatter())
	case "text":
		log.SetFormatter(textFormatter())
	default:
		return fmt.Errorf("log format '%s' is not recognized", config.LogFormat)
	}

	logLevel, err := log.ParseLevel(config.LogLevel)
	if err != nil {
		return fmt.Errorf("while setting log level: %s", err)
	}
	log.SetLevel(logLevel)

	log.Infof("ToBAC v%s (%s)", version.Version, version.Revision)

	k8sconfig, err := kubeclient.Config()
	if err != nil {
		return fmt.Errorf("while getting Kubernetes config: %s", err)
	}

	// Switch off TLS verification if needed
	if config.APIServerInsecureTLS {
		k8sconfig.TLSClientConfig.Insecure = true
		k8sconfig.TLSClientConfig.CAFile = ""
	}

	kubeClient, err = kubeclient.New(k8sconfig)
	if err != nil {
		return fmt.Errorf("while setting up Kubernetes client: %s", err)
	}

	tlsConfig, err := configTLS(*config)
	if err != nil {
		return fmt.Errorf("while setting up TLS: %s", err)
	}

	dur, err := time.ParseDuration(config.AzureSyncInterval)
	if err != nil {
		return fmt.Errorf("invalid sync interval: %s", err)
	}

	timeout, err := time.ParseDuration(config.AzureTimeout)
	if err != nil {
		return fmt.Errorf("invalid query timeout: %s", err)
	}

	log.Infof("Synchronizing team groups against Azure AD every %s", config.AzureSyncInterval)
	log.Infof("Cluster administrator groups: %+v", config.ClusterAdmins)
	log.Infof("Service user templates: %+v", config.ServiceUserTemplates)

	go teams.Sync(dur, timeout)
	go metrics.Serve(":8080", "/metrics", "/ready", "/alive")

	http.HandleFunc("/", serve)
	server := &http.Server{
		Addr:      ":8443",
		TLSConfig: tlsConfig,
	}
	server.ListenAndServeTLS("", "")

	log.Info("Shutting down cleanly.")

	return nil
}

func main() {
	err := run()
	if err != nil {
		log.Errorf("Fatal error: %s", err)
		os.Exit(1)
	}
}
