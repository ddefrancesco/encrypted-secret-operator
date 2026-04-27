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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

/* func (r *EncryptedSecretReconciler) Reconcile(
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
	rotation, err := rotationNeeded(es, secret)

	if err != nil {
		return ctrl.Result{}, err
	}
	if rotation {

		secret, err := r.createNewVersion(ctx, es)

		if err != nil {
			return ctrl.Result{}, err
		}

		r.updateAliasSecret(ctx, es, secret)

		r.restartTargets(ctx, es)

		r.cleanupOldVersions(ctx, es)

		es.Status.CurrentVersion++
		es.Status.LastRotation = metav1.Now()

		r.Status().Update(ctx, es)
	}

	return r.handleRotation(ctx, es, secret)
}
*/

func (r *EncryptedSecretReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {

	es := &securityv1alpha1.EncryptedSecret{}

	err := r.Get(ctx, req.NamespacedName, es)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1️⃣ TTL check
	if isExpired(es) {
		return r.handleDeletion(ctx, es)
	}

	// 2️⃣ Recupera alias secret (attivo)
	alias := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      es.Name,
		Namespace: es.Namespace,
	}, alias)

	aliasExists := err == nil

	// 3️⃣ Calcola hash dello spec
	specHash := hashData(es.Spec.Data)

	// 4️⃣ Verifica se serve cifrare (spec change o prima creazione)
	needEncrypt := false

	if !aliasExists {
		needEncrypt = true
	} else if alias.Annotations["spec-hash"] != specHash {
		needEncrypt = true
	}

	// 5️⃣ Se serve, chiama API crypto
	var newCipher map[string]string
	var newData map[string][]byte
	var newCipherHash string

	if needEncrypt {

		newCipher, err = crypto.Encrypt(
			es.Spec.CryptoEndpoint,
			es.Spec.Data,
		)

		if err != nil {
			return ctrl.Result{}, err
		}

		newData = map[string][]byte{}
		for k, v := range newCipher {
			newData[k] = []byte(v)
		}

		newCipherHash = hashBytes(newData)
	}

	// 6️⃣ Controllo rotazione temporale
	rotationDue := false

	if es.Spec.RotationInterval != "" {

		interval, err := time.ParseDuration(es.Spec.RotationInterval)
		if err != nil {
			return ctrl.Result{}, err
		}

		next := es.Status.LastRotation.Add(interval)

		if time.Now().After(next) {
			rotationDue = true
		}
	}

	// 7️⃣ Decisione finale
	rotate := false

	if !aliasExists {
		rotate = true
	} else if needEncrypt {

		oldCipherHash := alias.Annotations["cipher-hash"]

		// 🔥 pattern chiave: confronta ciphertext
		if oldCipherHash != newCipherHash {
			rotate = true
		}

	} else if rotationDue {
		rotate = true
	}

	// 8️⃣ Rotazione
	if rotate {

		// Se non abbiamo già cifrato (caso rotation only), cifriamo ora
		if newData == nil {

			newCipher, err = crypto.Encrypt(
				es.Spec.CryptoEndpoint,
				es.Spec.Data,
			)
			if err != nil {
				return ctrl.Result{}, err
			}

			newData = map[string][]byte{}
			for k, v := range newCipher {
				newData[k] = []byte(v)
			}

			newCipherHash = hashBytes(newData)
		}

		versionSecret, err := r.createNewVersionWithData(
			ctx, es, newData, specHash, newCipherHash,
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		err = r.updateAliasSecretWithMetadata(
			ctx, es, versionSecret, specHash, newCipherHash,
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		r.restartTargets(ctx, es)
		r.cleanupOldVersions(ctx, es)

		es.Status.CurrentVersion++
		es.Status.LastRotation = metav1.Now()
		es.Status.ActiveSecret = versionSecret.Name

		err = r.Status().Update(ctx, es)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// 9️⃣ Requeue per rotazione futura
	if es.Spec.RotationInterval != "" {

		interval, _ := time.ParseDuration(es.Spec.RotationInterval)

		return ctrl.Result{
			RequeueAfter: interval,
		}, nil
	}

	return ctrl.Result{}, nil
}
func (r *EncryptedSecretReconciler) createNewVersionWithData(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
	data map[string][]byte,
	specHash string,
	cipherHash string,
) (*corev1.Secret, error) {

	version := es.Status.CurrentVersion + 1

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-v%d", es.Name, version),
			Namespace: es.Namespace,
			Labels: map[string]string{
				"encrypted-secret": es.Name,
			},
			Annotations: map[string]string{
				"secret-version": strconv.Itoa(version),
				"spec-hash":      specHash,
				"cipher-hash":    cipherHash,
			},
		},
		Data: data,
	}

	err := r.Create(ctx, secret)
	return secret, err
}

func (r *EncryptedSecretReconciler) updateAliasSecretWithMetadata(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
	versionSecret *corev1.Secret,
	specHash string,
	cipherHash string,
) error {

	alias := &corev1.Secret{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      es.Name,
		Namespace: es.Namespace,
	}, alias)

	if errors.IsNotFound(err) {

		alias = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      es.Name,
				Namespace: es.Namespace,
			},
		}
	}

	alias.Data = versionSecret.Data

	if alias.Annotations == nil {
		alias.Annotations = map[string]string{}
	}

	alias.Annotations["spec-hash"] = specHash
	alias.Annotations["cipher-hash"] = cipherHash

	checksum := sha256.Sum256([]byte(fmt.Sprintf("%v", alias.Data)))
	alias.Annotations["checksum"] = hex.EncodeToString(checksum[:])

	return r.Update(ctx, alias)
}

func (r *EncryptedSecretReconciler) handleDeletion(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
) (ctrl.Result, error) {

	// Delete the alias secret
	alias := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      es.Name,
		Namespace: es.Namespace,
	}, alias)

	if err == nil {
		r.Delete(ctx, alias)
	}

	// Delete all version secrets
	list := &corev1.SecretList{}
	r.List(ctx, list,
		client.InNamespace(es.Namespace),
		client.MatchingLabels{
			"encrypted-secret": es.Name,
		})

	for i := range list.Items {
		r.Delete(ctx, &list.Items[i])
	}

	// Delete the EncryptedSecret resource
	r.Delete(ctx, es)

	return ctrl.Result{}, nil
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

func (r *EncryptedSecretReconciler) createNewVersion(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
) (*corev1.Secret, error) {

	version := es.Status.CurrentVersion + 1

	cipher, err := crypto.Encrypt(
		es.Spec.CryptoEndpoint,
		es.Spec.Data,
	)

	if err != nil {
		return nil, err
	}

	data := map[string][]byte{}
	for k, v := range cipher {
		data[k] = []byte(v)
	}

	cipherHash := hashBytes(data)
	specHash := hashData(es.Spec.Data)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-v%d", es.Name, version),
			Namespace: es.Namespace,
			Labels: map[string]string{
				"encrypted-secret": es.Name,
			},
			Annotations: map[string]string{
				"secret-version": strconv.Itoa(version),
				"spec-hash":      specHash,
				"cipher-hash":    cipherHash,
			},
		},
		Data: data,
	}

	err = r.Create(ctx, secret)
	return secret, err
}

func (r *EncryptedSecretReconciler) updateAliasSecret(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
	versionSecret *corev1.Secret,
) error {

	alias := &corev1.Secret{}

	err := r.Get(ctx,
		types.NamespacedName{
			Name:      es.Name,
			Namespace: es.Namespace,
		},
		alias,
	)

	if errors.IsNotFound(err) {

		alias = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      es.Name,
				Namespace: es.Namespace,
			},
		}
	}

	alias.Data = versionSecret.Data

	checksum := sha256.Sum256([]byte(fmt.Sprintf("%v", alias.Data)))

	if alias.Annotations == nil {
		alias.Annotations = map[string]string{}
	}

	alias.Annotations["checksum"] = hex.EncodeToString(checksum[:])

	return r.Update(ctx, alias)
}

func (r *EncryptedSecretReconciler) restartTargets(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
) {

	for _, target := range es.Spec.RestartTargets {

		parts := strings.Split(target, "/")

		if parts[0] == "deployment" {

			deploy := &appsv1.Deployment{}

			r.Get(ctx,
				types.NamespacedName{
					Name:      parts[1],
					Namespace: es.Namespace,
				},
				deploy)

			if deploy.Spec.Template.Annotations == nil {
				deploy.Spec.Template.Annotations = map[string]string{}
			}

			deploy.Spec.Template.Annotations["secret-rotated"] =
				time.Now().String()

			r.Update(ctx, deploy)
		}
	}
}

func (r *EncryptedSecretReconciler) cleanupOldVersions(
	ctx context.Context,
	es *securityv1alpha1.EncryptedSecret,
) {

	list := &corev1.SecretList{}

	r.List(ctx, list,
		client.InNamespace(es.Namespace),
		client.MatchingLabels{
			"encrypted-secret": es.Name,
		})

	if len(list.Items) <= es.Spec.MaxVersions {
		return
	}

	sort.Slice(list.Items, func(i, j int) bool {
		return list.Items[i].CreationTimestamp.Before(
			&list.Items[j].CreationTimestamp)
	})

	for i := 0; i < len(list.Items)-es.Spec.MaxVersions; i++ {
		r.Delete(ctx, &list.Items[i])
	}
}

func rotationNeededOld(
	es *securityv1alpha1.EncryptedSecret,
	currentSecret *corev1.Secret,
) (bool, error) {

	// 1. Nessuna versione ancora creata
	if es.Status.CurrentVersion == 0 {
		return true, nil
	}

	// 2. Rotation interval non configurato
	if es.Spec.RotationInterval == "" {
		return false, nil
	}

	interval, err := time.ParseDuration(es.Spec.RotationInterval)
	if err != nil {
		return false, err
	}

	// 3. controllo tempo
	nextRotation := es.Status.LastRotation.Add(interval)

	if time.Now().After(nextRotation) {
		return true, nil
	}

	// 4. controllo se i dati sono cambiati
	specHash := hashData(es.Spec.Data)

	if currentSecret.Annotations == nil {
		return true, nil
	}

	if currentSecret.Annotations["spec-hash"] != specHash {
		return true, nil
	}

	// 5. rotazione forzata
	if es.Annotations["force-rotation"] == "true" {
		return true, nil
	}

	return false, nil
}

func rotationNeeded(
	es *securityv1alpha1.EncryptedSecret,
	currentSecret *corev1.Secret,
	newCipher map[string][]byte,
) bool {

	// hash nuovo ciphertext
	newCipherHash := hashBytes(newCipher)

	if currentSecret.Annotations == nil {
		return true
	}

	oldCipherHash := currentSecret.Annotations["cipher-hash"]

	// 🔥 se il ciphertext NON cambia → NO rotazione
	if oldCipherHash == newCipherHash {
		return false
	}

	return true
}

func hashData(data map[string]string) string {

	keys := make([]string, 0, len(data))

	for k := range data {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	h := sha256.New()

	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte(data[k]))
	}

	return hex.EncodeToString(h.Sum(nil))
}

func hashBytes(data map[string][]byte) string {

	keys := make([]string, 0, len(data))

	for k := range data {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	h := sha256.New()

	for _, k := range keys {
		h.Write([]byte(k))
		h.Write(data[k])
	}

	return hex.EncodeToString(h.Sum(nil))
}
