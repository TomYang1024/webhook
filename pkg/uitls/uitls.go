package uitls

import (
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func WriteFile(filename string, data []byte) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}

// InitKubeClient
func InitKubeClient() (*kubernetes.Clientset, error) {
	var (
		err    error
		config *rest.Config
	)
	if config, err = rest.InClusterConfig(); err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}
