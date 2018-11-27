package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/golang/glog"
	"github.com/nais/tobac/pkg/teams"
	"k8s.io/api/admission/v1beta1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

var teamSyncInterval = 10 * time.Minute

// Config contains the server (the webhook) cert and key.
type Config struct {
	CertFile string
	KeyFile  string
}

// KubernetesResource represents any Kubernetes resource with standard object metadata structures.
type KubernetesResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

func (c *Config) addFlags() {
	flag.StringVar(&c.CertFile, "cert", c.CertFile, ""+
		"File containing the default x509 Certificate for HTTPS. (CA cert, if any, concatenated "+
		"after server cert).")
	flag.StringVar(&c.KeyFile, "key", c.KeyFile, ""+
		"File containing the default x509 private key matching --cert.")
}

func toAdmissionResponse(err error) *v1beta1.AdmissionResponse {
	return &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
}

func allowed(info authenticationv1.UserInfo, resource KubernetesResource) error {
	teamId := resource.Labels["team"]
	if len(teamId) == 0 {
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
	//
	// default deny
	return fmt.Errorf("default rule is to deny")
}

func admitCallback(ar v1beta1.AdmissionReview) *v1beta1.AdmissionResponse {
	resource := KubernetesResource{}
	r := bytes.NewReader(ar.Request.Object.Raw)
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&resource); err != nil {
		glog.Error(err)
		return nil
	}

	if ar.Request == nil {
		glog.Warning("Admission review request is nil")
		return nil
	}

	glog.Infof("Request '%s' from user '%s' in groups '%+v'", resource.SelfLink, ar.Request.UserInfo.Username, ar.Request.UserInfo.Groups)

	reviewResponse := v1beta1.AdmissionResponse{}
	err := allowed(ar.Request.UserInfo, resource)
	if err == nil {
		reviewResponse.Allowed = true
	} else {
		reviewResponse.Allowed = false
		glog.Infof("Denying request: %s", err)
	}

	return &reviewResponse
}

type admitFunc func(v1beta1.AdmissionReview) *v1beta1.AdmissionResponse

func serve(w http.ResponseWriter, r *http.Request, admit admitFunc) {
	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("contentType=%s, expect application/json", contentType)
		return
	}

	var reviewResponse *v1beta1.AdmissionResponse
	ar := v1beta1.AdmissionReview{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&ar); err != nil {
		glog.Error(err)
		reviewResponse = toAdmissionResponse(err)
	} else {
		reviewResponse = admit(ar)
	}

	response := v1beta1.AdmissionReview{}
	if reviewResponse != nil {
		response.Response = reviewResponse
		response.Response.UID = ar.Request.UID
	}

	glog.Infof("Sending admission response: %+v", response)

	encoder := json.NewEncoder(w)
	err := encoder.Encode(response)
	if err != nil {
		glog.Error(err)
	}
}

func serveAny(w http.ResponseWriter, r *http.Request) {
	serve(w, r, admitCallback)
}

func configTLS(config Config) *tls.Config {
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		glog.Fatal(err)
	}
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
	}
}

func main() {
	var config Config
	config.addFlags()
	flag.Parse()

	go teams.Sync(teamSyncInterval)

	http.HandleFunc("/", serveAny)
	server := &http.Server{
		Addr:      ":8443",
		TLSConfig: configTLS(config),
	}
	server.ListenAndServeTLS("", "")
}
