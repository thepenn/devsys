package k8s

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/thepenn/devsys/model"
	systemService "github.com/thepenn/devsys/service/system"
)

// Service provides helper methods to work with Kubernetes clusters registered as certificates.
type Service struct {
	system *systemService.Service
}

// New creates a new Kubernetes helper service.
func New(system *systemService.Service) *Service {
	return &Service{system: system}
}

// RESTConfig builds a rest.Config from the given certificate id.
func (s *Service) RESTConfig(ctx context.Context, certificateID int64) (*rest.Config, error) {
	if s.system == nil {
		return nil, fmt.Errorf("system service unavailable")
	}
	cert, err := s.system.GetCertificateWithSecrets(ctx, certificateID)
	if err != nil {
		return nil, err
	}
	if cert == nil {
		return nil, fmt.Errorf("certificate %d not found", certificateID)
	}
	if cert.Type != model.CertificateTypeKubernetes {
		return nil, fmt.Errorf("certificate %d is not kubernetes type", certificateID)
	}
	kubeCert, err := cert.AsKubernetesCertificate()
	if err != nil {
		return nil, err
	}
	if kubeCert.KubeConfig == "" {
		return nil, fmt.Errorf("kubeconfig is empty")
	}
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeCert.KubeConfig))
	if err != nil {
		return nil, err
	}
	cfg.QPS = 50
	cfg.Burst = 100
	return cfg, nil
}

// Clientset returns a typed Kubernetes clientset based on the stored kubeconfig.
func (s *Service) Clientset(ctx context.Context, certificateID int64) (kubernetes.Interface, error) {
	cfg, err := s.RESTConfig(ctx, certificateID)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}
