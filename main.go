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
	"github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"
	"k8s.io/api/admission/v1beta1"
	authenticationv1 "k8s.io/api/authentication/v1"
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
}

func DefaultConfig() *Config {
	return &Config{
		CertFile:             "/etc/tobac/tls.crt",
		KeyFile:              "/etc/tobac/tls.key",
		AzureSyncInterval:    "10m",
		ServiceUserTemplates: []string{"system:serviceaccount:default:serviceuser-%s"},
		LogFormat:            "text",
	}
}

var config = DefaultConfig()

// KubernetesResource represents any Kubernetes resource with standard object metadata structures.
type KubernetesResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "cert", c.CertFile, "File containing the x509 certificate for HTTPS.")
	flag.StringVar(&c.KeyFile, "key", c.KeyFile, "File containing the x509 private key.")
	flag.StringVar(&c.LogFormat, "log-format", c.LogFormat, "Log format, either 'json' or 'text'.")
	flag.StringVar(&c.AzureSyncInterval, "azure-sync-interval", c.AzureSyncInterval, "How often to synchronize the team list against Azure AD.")
	flag.StringSliceVar(&c.ServiceUserTemplates, "service-user-templates", c.ServiceUserTemplates, "List of Kubernetes users that will be granted access to resources. %s will be replaced by the team label.")
	flag.StringSliceVar(&c.ClusterAdmins, "cluster-admins", c.ClusterAdmins, "Commas-separated list of groups that are allowed to perform any action.")
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func contains(arr []string, target string) bool {
	for _, s := range arr {
		if target == s {
			return true
		}
	}
	return false
}

func allowed(info authenticationv1.UserInfo, previous, resource *KubernetesResource) error {
	teamID := resource.Labels["team"]
	if len(teamID) == 0 {
		return fmt.Errorf("object is not tagged with a team label")
	}

	// Allow if user is a cluster administrator
	for _, userGroup := range info.Groups {
		for _, adminGroup := range config.ClusterAdmins {
			if userGroup == adminGroup {
				return nil
			}
		}
	}

	// Deny if specified team does not exist
	team := teams.Get(resource.Labels["team"])
	if !team.Valid() {
		return fmt.Errorf("team '%s' does not exist in Azure AD", resource.Labels["team"])
	}

	// Deny if user does not belong to previous resource's team
	if previous != nil {
		label := previous.Labels["team"]

		// Allow users to claim previously unlabeled resources
		if len(label) > 0 {
			previousTeam := teams.Get(label)
			if !previousTeam.Valid() {
				return fmt.Errorf("team '%s' on existing resource does not exist in Azure AD", label)
			}
			if !contains(info.Groups, previousTeam.AzureUUID) {
				return fmt.Errorf("user '%s' has no access to team '%s'", info.Username, previousTeam.ID)
			}
		}
	}

	// Finally, allow if user exists in the specified team
	if contains(info.Groups, team.AzureUUID) {
		return nil
	}

	// If user does not exist in the specified team, try to match against service user templates.
	for _, template := range config.ServiceUserTemplates {
		allowedUser := fmt.Sprintf(template, team.ID)
		if info.Username == allowedUser {
			return nil
		}
	}

	// default deny
	return fmt.Errorf("user '%s' has no access to team '%s'", info.Username, teamID)
}

func decode(raw []byte) (*KubernetesResource, error) {
	k := &KubernetesResource{}
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
		logrus.Warning("Admission review request is nil")
		return nil
	}

	previous, err := decode(ar.Request.OldObject.Raw)
	if err != nil {
		logrus.Error(err)
		return nil
	}

	resource, err := decode(ar.Request.Object.Raw)
	if err != nil {
		logrus.Error(err)
		return nil
	}

	if len(resource.SelfLink) > 0 {
		logrus.Infof("Request '%s' from user '%s' in groups %+v", resource.SelfLink, ar.Request.UserInfo.Username, ar.Request.UserInfo.Groups)
	} else {
		logrus.Infof("Request from user '%s' in groups %+v", ar.Request.UserInfo.Username, ar.Request.UserInfo.Groups)
	}

	reviewResponse := v1beta1.AdmissionResponse{}
	err = allowed(ar.Request.UserInfo, previous, resource)
	if err == nil {
		reviewResponse.Allowed = true
		logrus.Infof("Request allowed.")
	} else {
		reviewResponse.Allowed = false
		reviewResponse.Result = &metav1.Status{
			Message: err.Error(),
		}
		logrus.Infof("Request denied: %s", err)
	}

	return &reviewResponse
}

type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		logrus.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&ar); err != nil {
		logrus.Error(err)
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
		logrus.Error(err)
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

func textFormatter() logrus.Formatter {
	return &logrus.TextFormatter{
		DisableTimestamp: false,
		FullTimestamp:    true,
	}
}

func run() error {
	config.addFlags()
	flag.Parse()

	switch config.LogFormat {
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{})
	case "text":
		logrus.SetFormatter(textFormatter())
	default:
		return fmt.Errorf("log format '%s' is not recognized", config.LogFormat)
	}

	tlsConfig, err := configTLS(*config)
	if err != nil {
		return fmt.Errorf("while setting up TLS: %s", err)
	}

	dur, err := time.ParseDuration(config.AzureSyncInterval)
	if err != nil {
		return fmt.Errorf("invalid sync interval: %s", err)
	}

	logrus.Info("ToBAC starting.")
	logrus.Infof("Synchronizing team groups against Azure AD every %s", config.AzureSyncInterval)
	logrus.Infof("Cluster administrator groups: %+v", config.ClusterAdmins)
	logrus.Infof("Service user templates: %+v", config.ServiceUserTemplates)

	go teams.Sync(dur)

	http.HandleFunc("/", serveAny)
	server := &http.Server{
		Addr:      ":8443",
		TLSConfig: tlsConfig,
	}
	server.ListenAndServeTLS("", "")

	logrus.Info("Shutting down cleanly.")

	return nil
}

func main() {
	err := run()
	if err != nil {
		logrus.Errorf("Fatal error: %s", err)
		os.Exit(1)
	}
}
