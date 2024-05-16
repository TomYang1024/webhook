package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	admissionV1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"
)

type WhSvrParam struct {
	Port     int
	CetrFile string
	KeyFile  string
}

type WebhookServer struct {
	Server           *http.Server
	WhiteListRegisry []string
}

type WebhookHandler interface {
	Handler(w http.ResponseWriter, r *http.Request)
}

type pathOperation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

const (
	AnnotationMutateKey = "io.k8s.admission-registry/mutate"
	AnnotationStatusKey = "io.k8s.admission-registry/status"
)

var (
	_             WebhookHandler = &WebhookServer{}
	runtimeScheme                = runtime.NewScheme()
	codeFactory                  = serializer.NewCodecFactory(runtimeScheme)
	deserializer                 = codeFactory.UniversalDeserializer()
)

func (whsvr *WebhookServer) Handler(w http.ResponseWriter, r *http.Request) {
	// TODO
	var body []byte
	if r.Body != nil {
		if res, err := io.ReadAll(r.Body); err == nil {
			body = res
		}
	}
	if len(body) == 0 {
		klog.Error("empty data body")
		http.Error(w, "empty data body", http.StatusBadRequest)
		return
	}
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		klog.Errorf("Content-Type: %s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}
	var (
		requestAdminssionReview admissionV1.AdmissionReview
		admissionResponse       *admissionV1.AdmissionResponse
	)

	//  序列化
	if _, _, err := deserializer.Decode(body, nil, &requestAdminssionReview); err != nil {
		klog.Errorf("Can't decode body: %v", err)
		admissionResponse = &admissionV1.AdmissionResponse{
			Result: &v1.Status{
				Message: err.Error(),
				Code:    http.StatusBadRequest,
			},
		}
	}
	switch {
	case r.URL.Path == "/validate":
		admissionResponse = whsvr.validate(&requestAdminssionReview)
	case r.URL.Path == "/mutate":
		admissionResponse = whsvr.mutate(&requestAdminssionReview)
	}

	responseAdmissionReview := admissionV1.AdmissionReview{}
	responseAdmissionReview.APIVersion = requestAdminssionReview.APIVersion
	responseAdmissionReview.Kind = requestAdminssionReview.Kind
	if admissionResponse != nil {
		responseAdmissionReview.Response = admissionResponse
		if requestAdminssionReview.Request != nil {
			responseAdmissionReview.Response.UID = requestAdminssionReview.Request.UID
		}
	}
	klog.Info(fmt.Sprintf("Sending response: %v", responseAdmissionReview.Response))

	respBytes, err := json.Marshal(responseAdmissionReview)
	if err != nil {
		klog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
		return
	}
	klog.Info("Ready to write reponse ...")
	if _, err := w.Write(respBytes); err != nil {
		klog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
		return
	}
}

func (whsvr *WebhookServer) validate(ar *admissionV1.AdmissionReview) *admissionV1.AdmissionResponse {
	req := ar.Request
	var (
		allowed bool = true
		code         = http.StatusOK
		message      = "Success"
	)
	klog.Info(fmt.Sprintf("AdmissionReview for Kind=%s, Namespace=%s Name=%s UID=%s patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, req.UID, req.Operation, req.UserInfo))

	var (
		pod corev1.Pod
	)
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		klog.Errorf("Can't unmarshal raw object: %v", err)
		allowed = false
		code = http.StatusBadRequest
		message = fmt.Sprintf("Can't unmarshal raw object: %v", err)
	}
	for _, container := range pod.Spec.Containers {
		var (
			whiteListed = false
		)
		for _, registry := range whsvr.WhiteListRegisry {
			if image := container.Image; !strings.HasPrefix(image, registry) {
				whiteListed = true
			}
		}
		if !whiteListed {
			allowed = false
			code = http.StatusForbidden
			message = fmt.Sprintf("Image %s is not allowed", container.Image)
			break
		}
	}
	return &admissionV1.AdmissionResponse{
		Allowed: allowed,
		Result: &v1.Status{
			Code:    int32(code),
			Message: message,
		},
	}
}

func (whsvr *WebhookServer) mutate(ar *admissionV1.AdmissionReview) *admissionV1.AdmissionResponse {
	// TODO Deployment Service
	req := ar.Request
	var (
		objectMeta v1.ObjectMeta
	)

	klog.Infof(
		"AdmissionReview for Kind=%s, Namespace=%s Name=%s UID=%s patchOperation=%v UserInfo=%v",
		req.Kind, req.Namespace, req.Name, req.UID, req.Operation, req.UserInfo,
	)
	switch req.Kind.Kind {
	case "Deployment":
		var deployment appsv1.Deployment
		if err := json.Unmarshal(req.Object.Raw, &deployment); err != nil {
			klog.Errorf("Can't unmarshal raw object: %v", err)
			return &admissionV1.AdmissionResponse{
				Result: &v1.Status{
					Code:    http.StatusBadRequest,
					Message: fmt.Sprintf("Can't unmarshal raw object: %v", err),
				},
			}
		}
		objectMeta = deployment.ObjectMeta
	case "Service":
		var service corev1.Service
		if err := json.Unmarshal(req.Object.Raw, &service); err != nil {
			klog.Errorf("Can't unmarshal raw object: %v", err)
			return &admissionV1.AdmissionResponse{
				Result: &v1.Status{
					Code:    http.StatusBadRequest,
					Message: fmt.Sprintf("Can't unmarshal raw object: %v", err),
				},
			}
		}
		objectMeta = service.ObjectMeta
	default:
		return &admissionV1.AdmissionResponse{
			Result: &v1.Status{
				Code:    http.StatusBadRequest,
				Message: fmt.Sprintf("Resource Kind=%s is not handled", req.Kind.Kind),
			},
		}
	}
	// 判断是否需要真执行
	if !whsvr.mutationRequired(&objectMeta) {
		klog.Infof("Skipping mutation for %s/%s", objectMeta.Namespace, objectMeta.Name)
		return &admissionV1.AdmissionResponse{
			Allowed: true,
		}
	}

	annotations := map[string]string{AnnotationStatusKey: "mutated"}

	var patch []pathOperation
	patch = append(patch, whsvr.mutateAnnotations(
		objectMeta.GetAnnotations(),
		annotations)...,
	)
	patchBytes, err := json.Marshal(patch)
	if err != nil {
		klog.Errorf("Can't marshal patch: %v", err)
		return &admissionV1.AdmissionResponse{
			Result: &v1.Status{
				Code:    http.StatusInternalServerError,
				Message: fmt.Sprintf("Can't marshal patch: %v", err),
			},
		}
	}

	return &admissionV1.AdmissionResponse{
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionV1.PatchType {
			pt := admissionV1.PatchTypeJSONPatch
			return &pt
		}(),
	}
}

// mutateAnnotations adds or updates annotations.
func (whsvr *WebhookServer) mutateAnnotations(target map[string]string, added map[string]string) (patch []pathOperation) {
	for k, value := range added {
		if target == nil || target[k] == "" {
			target = make(map[string]string)
			patch = append(patch, pathOperation{
				Op:   "add",
				Path: "/metadata/annotations",
				Value: map[string]string{
					k: value,
				},
			})
		}
		switch {
		case target == nil || target[k] == "":
			target = make(map[string]string)
			patch = append(patch, pathOperation{
				Op:   "add",
				Path: "/metadata/annotations",
				Value: map[string]string{
					k: value,
				},
			})
		default:
			patch = append(patch, pathOperation{
				Op:   "replace",
				Path: fmt.Sprintf("/metadata/annotations/%s", k),
				Value: map[string]string{
					k: value,
				},
			})
		}
	}
	return
}

// mutationRequired determines whether a mutation is required for the object.
func (whsvr *WebhookServer) mutationRequired(meta *v1.ObjectMeta) bool {
	annotations := meta.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	var required bool

	switch annotations[AnnotationMutateKey] {
	case "n", "no", "false", "off":
		required = false
	default:
		required = true
	}
	status := annotations[AnnotationStatusKey]
	if status == "mutated" {
		required = false
	}
	klog.Infof("Mutation policy for %s/%s: required=%v, status=%s",
		meta.Namespace,
		meta.Name,
		required,
		status,
	)
	return required
}
