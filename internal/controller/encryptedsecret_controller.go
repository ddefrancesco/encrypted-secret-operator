/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	securityv1alpha1 "github.com/copsds/encrypted-secret-operator/api/v1alpha1"
	"github.com/copsds/encrypted-secret-operator/internal/crypto"
)

// EncryptedSecretReconciler reconciles a EncryptedSecret object
type EncryptedSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=security.copsds.com,resources=encryptedsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.copsds.com,resources=encryptedsecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=security.copsds.com,resources=encryptedsecrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the EncryptedSecret object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile

func (r *EncryptedSecretReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)
	es := &securityv1alpha1.EncryptedSecret{}
	if isExpired(es) {

		secret := &corev1.Secret{}
		r.Get(ctx, types.NamespacedName{
			Name:      es.Status.SecretName,
			Namespace: es.Namespace,
		}, secret)

		r.Delete(ctx, secret)

		r.Delete(ctx, es)

		return ctrl.Result{}, nil
	}

	err := r.Get(ctx, req.NamespacedName, es)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	secretName := es.Name + "-cipher"

	secret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: es.Namespace,
	}, secret)

	if errors.IsNotFound(err) {

		return r.createSecret(ctx, es)

	}

	return r.handleRotation(ctx, es, secret)
}

func (r *EncryptedSecretReconciler) createSecret(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
) (ctrl.Result, error) {

	cipher, err := crypto.Encrypt(
		es.Spec.CryptoEndpoint,
		es.Spec.Data,
	)

	if err != nil {
		return ctrl.Result{}, err
	}

	data := map[string][]byte{}

	for k, v := range cipher {
		data[k] = []byte(v)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      es.Name + "-cipher",
			Namespace: es.Namespace,
		},
		Data: data,
	}

	ctrl.SetControllerReference(es, secret, r.Scheme)

	err = r.Create(ctx, secret)

	if err != nil {
		return ctrl.Result{}, err
	}

	es.Status.LastRotation = metav1.Now()

	r.Status().Update(ctx, es)

	return ctrl.Result{}, nil
}

func (r *EncryptedSecretReconciler) handleRotation(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
	secret *corev1.Secret,
) (ctrl.Result, error) {

	interval, _ := time.ParseDuration(es.Spec.RotationInterval)

	if interval == 0 {
		return ctrl.Result{}, nil
	}

	nextRotation := es.Status.LastRotation.Add(interval)

	if time.Now().Before(nextRotation) {

		return ctrl.Result{
			RequeueAfter: nextRotation.Sub(time.Now()),
		}, nil
	}

	cipher, err := crypto.Encrypt(
		es.Spec.CryptoEndpoint,
		es.Spec.Data,
	)

	if err != nil {
		return ctrl.Result{}, err
	}

	for k, v := range cipher {
		secret.Data[k] = []byte(v)
	}

	err = r.Update(ctx, secret)

	if err != nil {
		return ctrl.Result{}, err
	}

	es.Status.LastRotation = metav1.Now()

	r.Status().Update(ctx, es)

	return ctrl.Result{
		RequeueAfter: interval,
	}, nil
}

func isExpired(es *securityv1alpha1.EncryptedSecret) bool {

	if es.Spec.TTL == "" {
		return false
	}

	ttl, _ := time.ParseDuration(es.Spec.TTL)

	exp := es.CreationTimestamp.Add(ttl)

	return time.Now().After(exp)
}

// SetupWithManager sets up the controller with the Manager.
func (r *EncryptedSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1alpha1.EncryptedSecret{}).
		Named("encryptedsecret").
		Complete(r)
}
