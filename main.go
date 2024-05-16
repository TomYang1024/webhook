package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/tomyang/admission-registry/pkg/registry"
	"k8s.io/klog/v2"
)

func main() {
	var param registry.WhSvrParam
	flag.IntVar(&param.Port, "port", 443, "http server port")
	flag.StringVar(&param.CetrFile, "tlsCertFile", "/etc/webhook/certs/tls.crt", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&param.KeyFile, "tlsKeyFile", "/etc/webhook/certs/tls.key", "File containing the x509 private key to --tlsCertFile.")
	flag.Parse()
	certificate, err := tls.LoadX509KeyPair(param.CetrFile, param.KeyFile)
	if err != nil {
		klog.Errorf("Filed to load key pair: %v", err)
		return
	}
	whsrv := &registry.WebhookServer{
		Server: &http.Server{
			Addr: fmt.Sprintf(":%d", param.Port),
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{certificate},
			},
		},
		WhiteListRegisry: strings.Split(os.Getenv("WHITE_LIST_REGISRY"), ","),
	}

	// handler
	mux := http.NewServeMux()
	mux.HandleFunc("/validate", whsrv.Handler)
	mux.HandleFunc("/mutate", whsrv.Handler)
	whsrv.Server.Handler = mux
	go func() {
		if err := whsrv.Server.ListenAndServeTLS("", ""); err != nil {
			klog.Errorf("Failed to listen and serve webhook server: %v", err)
			return
		}
	}()
	klog.Info("Server started")
	gracfulShutdown(context.Background(), whsrv)
	klog.Info("Server stopped")
}

// gracfulShutdown 优雅关闭
func gracfulShutdown(ctx context.Context, whsrv *registry.WebhookServer) {
	signalCtx, stop := signal.NotifyContext(
		ctx,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGHUP,
	)
	<-signalCtx.Done()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer func() {
		cancel()
		stop()
		if err := whsrv.Server.Shutdown(ctx); err != nil {
			klog.Infof("Failed to shutdown webhook server: %v", err)
		}
	}()
}
