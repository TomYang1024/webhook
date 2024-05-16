package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/tomyang/admission-registry/pkg/uitls"
	admissionV1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
	subject := pkix.Name{
		Country:            []string{"CN"},
		Province:           []string{"Beijing"},
		Locality:           []string{"Beijing"},
		Organization:       []string{"tomyang2024.io"},
		OrganizationalUnit: []string{"tomyang2024"},
	}
	ca := x509.Certificate{
		SerialNumber: big.NewInt(2024),
		Subject:      subject,
		NotBefore:    time.Now(),                   //证书有效期开始
		NotAfter:     time.Now().AddDate(10, 0, 0), //证书有效期结束
		IsCA:         true,                         //是否是根证书
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth, //客户端认证
			x509.ExtKeyUsageServerAuth, //服务端认证
		},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, //证书用途
		BasicConstraintsValid: true,                                                  //基本的有效性约束
	}
	// 生成证书
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		log.Panic(err)
	}
	// 创建自签名的CA 证书
	caBytes, err := x509.CreateCertificate(
		rand.Reader,
		&ca,
		&ca,
		caPrivKey.PublicKey,
		caPrivKey,
	)
	if err != nil {
		log.Panic(err)
	}
	// 保存证书到文件
	caPEM := new(bytes.Buffer)
	if err := pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	}); err != nil {
		log.Panic(err)
	}
	dnsName := []string{
		"admission-registry",
		"admission-registry.default",
		"admission-registry.default.svc",
		"admission-registry.default.svc.cluster.local",
	}
	commonName := "admission-registry.default.svc"
	subject.CommonName = commonName
	cert := &x509.Certificate{
		DNSNames:     dnsName,
		SerialNumber: big.NewInt(2024),
		Subject:      subject,
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	// 生成服务端的私钥
	serverPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		log.Panic(err)
	}
	// 生成服务端的证书
	serveCertBytes, err := x509.CreateCertificate(
		rand.Reader,
		cert,
		&ca,
		&serverPrivKey.PublicKey,
		caPrivKey,
	)
	if err != nil {
		log.Panic(err)
	}

	serverCertPEM := new(bytes.Buffer)
	if err := pem.Encode(serverCertPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serveCertBytes,
	}); err != nil {
		log.Panic(err)
	}
	serverPrivKeyPEM := new(bytes.Buffer)
	if err := pem.Encode(serverPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey),
	}); err != nil {
		log.Panic(err)
	}
	// 保存证书和私钥到文件

	if err := os.MkdirAll("/etc/webhook/certs", 0666); err != nil {
		log.Panic(err)
	}
	if err := uitls.WriteFile("/etc/webhook/certs/tls.crt", serverCertPEM.Bytes()); err != nil {
		log.Panic(err)
	}
	if err := uitls.WriteFile("/etc/webhook/certs/tls.key", serverPrivKeyPEM.Bytes()); err != nil {
		log.Panic(err)
	}

	if err := CreateAdmissionWebhookConfigMap(caPEM); err != nil {
		log.Panic(err)
	}
}

func CreateAdmissionWebhookConfigMap(caCert *bytes.Buffer) error {
	clientSet, err := uitls.InitKubeClient()
	var (
		ctx = context.TODO()
	)
	if err != nil {
		return err
	}
	var (
		webhookNamespace, _ = os.LookupEnv("WEBHOOK_NAMESPACE")
		validateCfgName, _  = os.LookupEnv("VALIDATE_COFING")
		mutateCfgName, _    = os.LookupEnv("MUTATE_CONFIG")
		webhookService, _   = os.LookupEnv("WEBHOOK_SERVICE")
		validatePath, _     = os.LookupEnv("VALIDATE_PATH")
		mutatePath, _       = os.LookupEnv("MUTATE_PATH")
	)
	if validateCfgName != "" {
		validteConfig := &admissionV1.ValidatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: validateCfgName,
			},
			Webhooks: []admissionV1.ValidatingWebhook{
				{
					Name: "io.docker.admission-registry",
					ClientConfig: admissionV1.WebhookClientConfig{
						CABundle: caCert.Bytes(),
						Service: &admissionV1.ServiceReference{
							Namespace: webhookNamespace,
							Name:      webhookService,
							Path:      &validatePath,
						},
					},
					Rules: []admissionV1.RuleWithOperations{
						{
							Operations: []admissionV1.OperationType{
								admissionV1.Create,
							},
							Rule: admissionV1.Rule{
								APIGroups:   []string{""},
								APIVersions: []string{"v1"},
								Resources:   []string{"pods"},
							},
						},
					},
					AdmissionReviewVersions: []string{"v1"},
					SideEffects: func() *admissionV1.SideEffectClass {
						var (
							sideEffect = admissionV1.SideEffectClassNone
						)
						return &sideEffect
					}(),
				},
			},
		}
		validteAdmissionClient := clientSet.
			AdmissionregistrationV1().
			ValidatingWebhookConfigurations()
		if _, err := validteAdmissionClient.Get(ctx, validateCfgName, metav1.GetOptions{}); err != nil {
			if errors.IsNotFound(err) {
				if _, err := validteAdmissionClient.Create(ctx, validteConfig, metav1.CreateOptions{}); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		if _, err := validteAdmissionClient.Update(ctx, validteConfig, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}

	if mutateCfgName != "" {
		mutateConfig := &admissionV1.MutatingWebhookConfiguration{
			ObjectMeta: metav1.ObjectMeta{
				Name: validateCfgName,
			},
			Webhooks: []admissionV1.MutatingWebhook{
				{
					Name: "io.docker.admission-registry-mutate",
					ClientConfig: admissionV1.WebhookClientConfig{
						CABundle: caCert.Bytes(),
						Service: &admissionV1.ServiceReference{
							Namespace: webhookNamespace,
							Name:      webhookService,
							Path:      &mutatePath,
						},
					},
					Rules: []admissionV1.RuleWithOperations{
						{
							Operations: []admissionV1.OperationType{
								admissionV1.Create,
							},
							Rule: admissionV1.Rule{
								APIGroups:   []string{"apps", ""},
								APIVersions: []string{"v1"},
								Resources:   []string{"deployments", "services"},
							},
						},
					},
					AdmissionReviewVersions: []string{"v1"},
					SideEffects: func() *admissionV1.SideEffectClass {
						var (
							sideEffect = admissionV1.SideEffectClassNone
						)
						return &sideEffect
					}(),
				},
			},
		}
		mutateAdmissionClient := clientSet.
			AdmissionregistrationV1().
			MutatingWebhookConfigurations()
		if _, err := mutateAdmissionClient.Get(ctx, validateCfgName, metav1.GetOptions{}); err != nil {
			if errors.IsNotFound(err) {
				if _, err := mutateAdmissionClient.Create(ctx, mutateConfig, metav1.CreateOptions{}); err != nil {
					return err
				}
			} else {
				return err
			}
		}
		if _, err := mutateAdmissionClient.Update(ctx, mutateConfig, metav1.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}
