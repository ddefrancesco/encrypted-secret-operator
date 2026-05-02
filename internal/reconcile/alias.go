package reconcile

import (
	"context"
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Aggiorna o crea alias Secret
func UpdateAlias(
	ctx context.Context,
	k8sClient client.Client,
	name string,
	namespace string,
	data map[string][]byte,
	secretType corev1.SecretType,
	annotations map[string]string,
) error {
	log := ctrl.LoggerFrom(ctx)
	alias := &corev1.Secret{}

	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, alias)

	if err != nil {
		alias = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name,
				Namespace:   namespace,
				Annotations: annotations,
			},
			Data: data,
			Type: secretType,
		}
		log.Info("Secret alias creating", "secretName", name)
		return k8sClient.Create(ctx, alias)
	}

	alias.Data = data
	alias.Type = secretType

	if alias.Annotations == nil {
		alias.Annotations = map[string]string{}
	}

	maps.Copy(alias.Annotations, annotations)
	log.Info("Secret alias updating", "secretName", name)
	return k8sClient.Update(ctx, alias)
}
