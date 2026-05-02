package reconcile

import (
	"context"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func CleanupOldVersions(
	ctx context.Context,
	k8sClient client.Client,
	namespace string,
	labelKey string,
	labelValue string,
	maxVersions int,
) error {

	if maxVersions == 0 {
		return nil
	}

	list := &corev1.SecretList{}

	err := k8sClient.List(ctx,
		list,
		client.InNamespace(namespace),
		client.MatchingLabels{
			labelKey: labelValue,
		},
	)
	if err != nil {
		return err
	}
	// Se sotto soglia → niente da fare
	if len(list.Items) <= maxVersions {
		return nil
	}
	// Ordina per CreationTimestamp (più vecchi prima)
	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].CreationTimestamp.Before(
			&list.Items[j].CreationTimestamp)
	})

	toDelete := len(list.Items) - maxVersions

	for i := 0; i < toDelete; i++ {
		_ = k8sClient.Delete(ctx, &list.Items[i])
	}

	return nil
}
