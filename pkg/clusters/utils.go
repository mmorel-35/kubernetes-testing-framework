package clusters

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/google/uuid"
	"github.com/kong/kubernetes-testing-framework/pkg/utils/kubernetes/generators"
	corev1 "k8s.io/api/core/v1"
	extv1beta1 "k8s.io/api/extensions/v1beta1"
	netv1 "k8s.io/api/networking/v1"
	netv1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// -----------------------------------------------------------------------------
// Resource Labels
// -----------------------------------------------------------------------------

const (
	// TestResourceLabel is a label used on any resources to indicate that they
	// were created as part of a testing run and can be cleaned up in bulk based
	// on the value provided to the label.
	TestResourceLabel = "created-by-ktf"
)

// -----------------------------------------------------------------------------
// Helper Functions
// -----------------------------------------------------------------------------

// DeployIngress is a helper and function to deploy an Ingress object to a cluster handling
// the version of the Ingress object for the caller so they don't have to.
// TODO: once we stop supporting old Kubernetes versions <1.19 we can remove this.
func DeployIngress(ctx context.Context, c Cluster, namespace string, ingress runtime.Object) (err error) {
	switch obj := ingress.(type) {
	case *netv1.Ingress:
		_, err = c.Client().NetworkingV1().Ingresses(namespace).Create(ctx, obj, metav1.CreateOptions{})
	case *netv1beta1.Ingress:
		_, err = c.Client().NetworkingV1beta1().Ingresses(namespace).Create(ctx, obj, metav1.CreateOptions{})
	case *extv1beta1.Ingress:
		_, err = c.Client().ExtensionsV1beta1().Ingresses(namespace).Create(ctx, obj, metav1.CreateOptions{})
	default:
		err = fmt.Errorf("%T is not a supported ingress type", ingress)
	}
	return
}

// DeleteIngress is a helper and function to delete an Ingress object to a cluster handling
// the version of the Ingress object for the caller so they don't have to.
// TODO: once we stop supporting old Kubernetes versions <1.19 we can remove this.
func DeleteIngress(ctx context.Context, c Cluster, namespace string, ingress runtime.Object) (err error) {
	switch obj := ingress.(type) {
	case *netv1.Ingress:
		err = c.Client().NetworkingV1().Ingresses(namespace).Delete(ctx, obj.Name, metav1.DeleteOptions{})
	case *netv1beta1.Ingress:
		err = c.Client().NetworkingV1beta1().Ingresses(namespace).Delete(ctx, obj.Name, metav1.DeleteOptions{})
	case *extv1beta1.Ingress:
		err = c.Client().ExtensionsV1beta1().Ingresses(namespace).Delete(ctx, obj.Name, metav1.DeleteOptions{})
	default:
		err = fmt.Errorf("%T is not a supported ingress type", ingress)
	}
	return
}

// GetIngressLoadbalancerStatus is a partner to the above DeployIngress function which will
// given an Ingress object provided by the caller determine the version and pull a fresh copy
// of the current LoadBalancerStatus for that Ingress object without the caller needing to be
// aware of which version of Ingress they're using.
// TODO: once we stop supporting old Kubernetes versions <1.19 we can remove this.
func GetIngressLoadbalancerStatus(ctx context.Context, c Cluster, namespace string, ingress runtime.Object) (*corev1.LoadBalancerStatus, error) {
	switch obj := ingress.(type) {
	case *netv1.Ingress:
		refresh, err := c.Client().NetworkingV1().Ingresses(namespace).Get(ctx, obj.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &refresh.Status.LoadBalancer, nil
	case *netv1beta1.Ingress:
		refresh, err := c.Client().NetworkingV1beta1().Ingresses(namespace).Get(ctx, obj.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &refresh.Status.LoadBalancer, nil
	case *extv1beta1.Ingress:
		refresh, err := c.Client().ExtensionsV1beta1().Ingresses(namespace).Get(ctx, obj.Name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		return &refresh.Status.LoadBalancer, nil
	default:
		return nil, fmt.Errorf("%T is not a supported ingress type", ingress)
	}
}

// CreateNamespace creates a new namespace in the given cluster provided a name.
func CreateNamespace(ctx context.Context, cluster Cluster, namespace string) error {
	nsName := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	_, err := cluster.Client().CoreV1().Namespaces().Create(context.Background(), nsName, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

// TempKubeconfig produces a kubeconfig tempfile given a cluster.
// the caller is responsible for cleaning up the file if they want it removed.
func TempKubeconfig(cluster Cluster) (*os.File, error) {
	// generate a kubeconfig from the cluster rest.Config
	kubeconfigBytes, err := generators.NewKubeConfigForRestConfig(cluster.Name(), cluster.Config())
	if err != nil {
		return nil, err
	}

	// create a tempfile to store the kubeconfig contents
	kubeconfig, err := ioutil.TempFile(os.TempDir(), fmt.Sprintf("-kubeconfig-%s", cluster.Name()))
	if err != nil {
		return nil, err
	}

	// write the contents
	c, err := kubeconfig.Write(kubeconfigBytes)
	if err != nil {
		return nil, err
	}

	// validate the file integrity
	if c != len(kubeconfigBytes) {
		return nil, fmt.Errorf("failed to write kubeconfig to %s (only %d/%d written)", kubeconfig.Name(), c, len(kubeconfigBytes))
	}

	return kubeconfig, nil
}

// GenerateNamespace creates a transient testing namespace given the cluster to create
// it on and a creator ID. The namespace will be given a UUID for a name, and the creatorID
// will be applied to the TestResourceLabel for automated cleanup.
func GenerateNamespace(ctx context.Context, cluster Cluster, creatorID string) (*corev1.Namespace, error) {
	if creatorID == "" {
		return nil, fmt.Errorf(`empty string "" is not a valid creator ID`)
	}

	name := uuid.NewString()
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				TestResourceLabel: creatorID,
			},
		},
	}

	return cluster.Client().CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
}

// CleanupGeneratedResources cleans up all resources created by the given creator ID.
func CleanupGeneratedResources(ctx context.Context, cluster Cluster, creatorID string) error {
	if creatorID == "" {
		return fmt.Errorf(`empty string "" is not a valid creator ID`)
	}

	listOpts := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", TestResourceLabel, creatorID),
	}

	namespaceList, err := cluster.Client().CoreV1().Namespaces().List(ctx, listOpts)
	if err != nil {
		return err
	}

	namespacesToCleanup := make(map[string]*corev1.Namespace)
	for i := 0; i < len(namespaceList.Items); i++ {
		namespace := &(namespaceList.Items[i])
		namespacesToCleanup[namespace.Name] = namespace
	}

	for len(namespacesToCleanup) > 0 {
		select {
		case <-ctx.Done():
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("context completed with error while waiting for cleanup: %w", err)
			}
			return fmt.Errorf("context completed while waiting for cleanup")
		default:
			for _, namespace := range namespaceList.Items {
				if err := cluster.Client().CoreV1().Namespaces().Delete(ctx, namespace.Name, metav1.DeleteOptions{}); err != nil {
					if errors.IsNotFound(err) {
						delete(namespacesToCleanup, namespace.Name)
					} else {
						return fmt.Errorf("failed to delete namespace resource %s: %w", namespace.Name, err)
					}
				}
			}
		}
	}

	return nil
}
