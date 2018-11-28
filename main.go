package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"time"

	"github.com/nais/tobac/pkg/teams"
	flag "github.com/spf13/pflag"
	"k8s.io/api/admission/v1beta1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var teamSyncInterval = 10 * time.Minute

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile      string
	KeyFile       string
	ClusterAdmins []string
}

func DefaultConfig() *Config {
	return &Config{
		CertFile: "/etc/tobac/tls.crt",
		KeyFile:  "/etc/tobac/tls.key",
	}
}

// KubernetesResource represents any Kubernetes resource with standard object metadata structures.
type KubernetesResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "cert", c.CertFile, "File containing the x509 certificate for HTTPS.")
	flag.StringVar(&c.KeyFile, "key", c.KeyFile, "File containing the x509 private key.")
	flag.StringSlice("cluster-admins", c.ClusterAdmins, "Commas-separated list of groups that are allowed to perform any action.")
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func allowed(info authenticationv1.UserInfo, resource KubernetesResource) error {
	teamID := resource.Labels["team"]
	if len(teamID) == 0 {
		return fmt.Errorf("object is not tagged with a team label")
	}

	team := teams.Get(resource.Labels["team"])
	if !team.Valid() {
		return fmt.Errorf("team '%s' does not exist in Azure AD", resource.Labels["team"])
	}

	// if clusterAdmin: allow
	//
	// if update and not in old team label group: deny
	//
	// if in team label group: allow
	for _, azureUUID := range info.Groups {
		if azureUUID == team.AzureUUID {
			return nil
		}
	}

	// default deny
	return fmt.Errorf("default rule is to deny")
}

func admitCallback(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	resource := KubernetesResource{}
	r := bytes.NewReader(ar.Request.Object.Raw)
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&resource); err != nil {
		logrus.Error(err)
		return nil
	}

	if ar.Request == nil {
		logrus.Warning("Admission review request is nil")
		return nil
	}

	logrus.Infof("Request '%s' from user '%s' in groups '%+v'", resource.SelfLink, ar.Request.UserInfo.Username, ar.Request.UserInfo.Groups)

	reviewResponse := v1beta1.AdmissionResponse{}
	err := allowed(ar.Request.UserInfo, resource)
	if err == nil {
		reviewResponse.Allowed = true
	} else {
		reviewResponse.Allowed = false
		reviewResponse.Result = &metav1.Status{
			Message: fmt.Sprintf("Unable to complete request, %s", err.Error()),
		}
		logrus.Infof("Denying request: %s", err)
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

	logrus.Infof("Sending admission response: %+v", response)

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

func run() error {
	config := DefaultConfig()
	config.addFlags()
	flag.Parse()

	tls, err := configTLS(*config)
	if err != nil {
		return err
	}

	go teams.Sync(teamSyncInterval)

	http.HandleFunc("/", serveAny)
	server := &http.Server{
		Addr:      ":8443",
		TLSConfig: tls,
	}
	server.ListenAndServeTLS("", "")

	return nil
}

func main() {
	err := run()
	if err != nil {
		logrus.Errorf("Fatal error: %s", err)
		os.Exit(1)
	}
}
