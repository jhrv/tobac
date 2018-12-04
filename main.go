package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/nais/tobac/pkg/teams"
	"github.com/nais/tobac/pkg/tobac"
	"github.com/nais/tobac/pkg/version"
	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile             string
	KeyFile              string
	LogFormat            string
	AzureSyncInterval    string
	ServiceUserTemplates []string
	ClusterAdmins        []string
	LogLevel             string
}

func DefaultConfig() *Config {
	return &Config{
		CertFile:             "/etc/tobac/tls.crt",
		KeyFile:              "/etc/tobac/tls.key",
		AzureSyncInterval:    "10m",
		ServiceUserTemplates: []string{"system:serviceaccount:default:serviceuser-%s"},
		LogFormat:            "text",
		LogLevel:             "info",
	}
}

var config = DefaultConfig()

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "cert", c.CertFile, "File containing the x509 certificate for HTTPS.")
	flag.StringVar(&c.KeyFile, "key", c.KeyFile, "File containing the x509 private key.")
	flag.StringVar(&c.LogFormat, "log-format", c.LogFormat, "Log format, either 'json' or 'text'.")
	flag.StringVar(&c.AzureSyncInterval, "azure-sync-interval", c.AzureSyncInterval, "How often to synchronize the team list against Azure AD.")
	flag.StringSliceVar(&c.ServiceUserTemplates, "service-user-templates", c.ServiceUserTemplates, "List of Kubernetes users that will be granted access to resources. %s will be replaced by the team label.")
	flag.StringSliceVar(&c.ClusterAdmins, "cluster-admins", c.ClusterAdmins, "Commas-separated list of groups that are allowed to perform any action.")
	flag.StringVar(&c.LogLevel, "log-level", c.LogLevel, "Logging verbosity level.")
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
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

func admitCallback(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	if ar.Request == nil {
		log.Warning("Admission review request is nil")
		return nil
	}

	previous, err := decode(ar.Request.OldObject.Raw)
	if err != nil {
		log.Error(err)
		return nil
	}

	resource, err := decode(ar.Request.Object.Raw)
	if err != nil {
		log.Error(err)
		return nil
	}

	if resource != nil && len(resource.SelfLink) > 0 {
		log.Infof("Request '%s' from user '%s' in groups %+v", resource.SelfLink, ar.Request.UserInfo.Username, ar.Request.UserInfo.Groups)
	} else {
		log.Infof("Request from user '%s' in groups %+v", ar.Request.UserInfo.Username, ar.Request.UserInfo.Groups)
	}

	log.Tracef("resource/old: %+v", resource)
	log.Tracef("resource/new: %+v", previous)

	response := tobac.Allowed(
		tobac.Request{
			UserInfo:             ar.Request.UserInfo,
			ExistingResource:     previous,
			SubmittedResource:    resource,
			ClusterAdmins:        config.ClusterAdmins,
			ServiceUserTemplates: config.ServiceUserTemplates,
			TeamProvider:         teams.Get,
		},
	)

	reviewResponse := v1beta1.AdmissionResponse{
		Allowed: response.Allowed,
		Result: &metav1.Status{
			Message: response.Reason,
		},
	}

	if response.Allowed {
		log.Infof("Request allowed: %s", response.Reason)
	} else {
		log.Warningf("Request denied: %s", response.Reason)
	}

	return &reviewResponse
}

type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		log.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&ar); err != nil {
		log.Error(err)
		reviewResponse = toAdmissionResponse(err)
	} else {
		reviewResponse = admit(ar)
	}

	response := v1beta1.AdmissionReview{}
	if reviewResponse != nil {
		response.Response = reviewResponse
		response.Response.UID = ar.Request.UID
	}

	encoder := json.NewEncoder(w)
	err := encoder.Encode(response)
	if err != nil {
		log.Error(err)
	}
}

func serveAny(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitCallback)
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

func run() error {
	config.addFlags()
	flag.Parse()

	switch config.LogFormat {
	case "json":
		log.SetFormatter(&log.JSONFormatter{})
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

	tlsConfig, err := configTLS(*config)
	if err != nil {
		return fmt.Errorf("while setting up TLS: %s", err)
	}

	dur, err := time.ParseDuration(config.AzureSyncInterval)
	if err != nil {
		return fmt.Errorf("invalid sync interval: %s", err)
	}

	log.Infof("ToBAC v%s (%s)", version.Version, version.Revision)
	log.Infof("Synchronizing team groups against Azure AD every %s", config.AzureSyncInterval)
	log.Infof("Cluster administrator groups: %+v", config.ClusterAdmins)
	log.Infof("Service user templates: %+v", config.ServiceUserTemplates)

	go teams.Sync(dur)

	http.HandleFunc("/", serveAny)
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
