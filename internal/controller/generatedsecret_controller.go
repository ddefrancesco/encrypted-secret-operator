package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	securityv1alpha1 "github.com/copsds/encrypted-secret-operator/api/v1alpha1"
	"github.com/copsds/encrypted-secret-operator/internal/crypto"
	"github.com/copsds/encrypted-secret-operator/internal/hash"
	"github.com/copsds/encrypted-secret-operator/internal/reconcile"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type GeneratedSecretReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=security.copsds.com,resources=generatedsecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=security.copsds.com,resources=generatedsecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=security.copsds.com,resources=generatedsecrets/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

func (r *GeneratedSecretReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	gs := &securityv1alpha1.GeneratedSecret{}

	err := r.Get(ctx, req.NamespacedName, gs)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log.Info("Reconciling GeneratedSecret", "name", gs.Name)
	// 1️⃣ trigger check
	need, err := generationNeeded(gs)
	log.Info("Generation needed?", "need", need)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !need {
		return ctrl.Result{}, nil
	}

	//idempotencyKey := computeIdempotencyKey(gs)
	// 2️⃣ spec hash + generation hash
	specHash := hash.ComputeSpecHash(gs.Spec.Type, gs.Spec.Parameters)
	version := gs.Status.CurrentVersion + 1
	genHash := hash.ComputeGenerationHash(specHash, version)

	// 2️⃣ bis chiamata API generazione
	log.Info("Calling generation API", "endpoint", gs.Spec.Endpoint.Address)
	resp, err := crypto.Generate(
		gs.Spec.Endpoint,
		gs.Spec.Type,
		gs.Spec.Parameters,
		genHash,
	)
	log.Info("Generation API response received")
	if err != nil {
		return ctrl.Result{}, err
	}
	log.Info("Generation API response", "metadata", resp.Metadata)
	log.Info("Generation API response", "data", resp.Data)
	byteData := map[string][]byte{}
	for k, v := range resp.Data {
		byteData[k] = []byte(v)
	}

	cipherHash := hash.HashBytes(byteData)

	annotations := map[string]string{
		reconcile.AnnotationSpecHash:       specHash,
		reconcile.AnnotationGenerationHash: genHash,
		reconcile.AnnotationCipherHash:     cipherHash,
		reconcile.AnnotationChecksum:       hash.ComputeChecksum(byteData),
	}
	// 3️⃣ versioning
	secretName := fmt.Sprintf("%s-v%d", gs.Name, version)
	log.Info("Created new Secret version", "secretName", secretName)

	// secret := &corev1.Secret{
	// 	ObjectMeta: metav1.ObjectMeta{
	// 		Name:      secretName,
	// 		Namespace: gs.Namespace,
	// 		Labels: map[string]string{
	// 			"generated-secret": gs.Name,
	// 		},
	// 		Annotations: annotations,
	// 	},
	// 	Data: byteData,
	// }
	// log.Info("Creating new Secret", "secretName", secretName)
	// err = r.Create(ctx, secret)
	// if err != nil {
	// 	return ctrl.Result{}, err
	// }

	// log.Info("Secret created successfully", "secretName", secretName)
	// 4️⃣ aggiornamento alias
	log.Info("Creating/Updating alias Secret", "aliasName", gs.Name)
	reconcile.UpdateAlias(
		ctx,
		r.Client,
		gs.Name,
		gs.Namespace,
		byteData,
		corev1.SecretTypeOpaque,
		annotations,
	)
	log.Info("Alias Secret created/updated successfully", "aliasName", gs.Name)
	// 5️⃣ cleanup
	log.Info("Cleaning up old versions if needed", "maxVersions", gs.Spec.MaxVersions)
	reconcile.CleanupOldVersions(
		ctx,
		r.Client,
		gs.Namespace,
		"generated-secret",
		gs.Name,
		gs.Spec.MaxVersions,
	)
	log.Info("Old versions cleanup completed if needed")
	// 6️⃣ update status
	log.Info("Updating GeneratedSecret status")
	gs.Status.CurrentVersion++
	gs.Status.LastGeneration = metav1.Now()
	gs.Status.ObservedHash = hashStructStable(gs.Spec.Parameters)

	if err := r.Status().Update(ctx, gs); err != nil {
		log.Error(err, "failed to update GeneratedSecret status")
		return ctrl.Result{}, err
	}
	log.Info("GeneratedSecret status updated successfully")
	log.Info("Reconciliation completed successfully", "generatedSecret", gs.Name)
	return ctrl.Result{}, nil
}

// func (r *GeneratedSecretReconciler) cleanupOldVersions(
// 	ctx context.Context,
// 	gs *securityv1alpha1.GeneratedSecret,
// ) error {

// 	// Se non configurato → niente cleanup
// 	if gs.Spec.MaxVersions == 0 {
// 		return nil
// 	}

// 	secretList := &corev1.SecretList{}

// 	err := r.List(ctx,
// 		secretList,
// 		client.InNamespace(gs.Namespace),
// 		client.MatchingLabels{
// 			"generated-secret": gs.Name,
// 		},
// 	)

// 	if err != nil {
// 		return err
// 	}

// 	// Se sotto soglia → niente da fare
// 	if len(secretList.Items) <= gs.Spec.MaxVersions {
// 		return nil
// 	}

// 	// Ordina per CreationTimestamp (più vecchi prima)
// 	sort.Slice(secretList.Items, func(i, j int) bool {
// 		return secretList.Items[i].CreationTimestamp.Before(
// 			&secretList.Items[j].CreationTimestamp,
// 		)
// 	})

// 	toDelete := len(secretList.Items) - gs.Spec.MaxVersions

// 	for i := 0; i < toDelete; i++ {

// 		secret := secretList.Items[i]

// 		err := r.Delete(ctx, &secret)
// 		if err != nil && !errors.IsNotFound(err) {
// 			return err
// 		}
// 	}

// 	return nil
// }

// func (r *GeneratedSecretReconciler) updateAlias(
// 	ctx context.Context,
// 	gs *securityv1alpha1.GeneratedSecret,
// 	versionSecret *corev1.Secret,
// ) error {

// 	aliasName := gs.Name

// 	alias := &corev1.Secret{}

// 	err := r.Get(ctx, types.NamespacedName{
// 		Name:      aliasName,
// 		Namespace: gs.Namespace,
// 	}, alias)

// 	// Se non esiste → crealo
// 	if errors.IsNotFound(err) {

// 		newAlias := &corev1.Secret{
// 			ObjectMeta: metav1.ObjectMeta{
// 				Name:      aliasName,
// 				Namespace: gs.Namespace,
// 				Labels: map[string]string{
// 					"generated-secret": gs.Name,
// 				},
// 			},
// 			Data: versionSecret.Data,
// 			Type: versionSecret.Type,
// 		}

// 		// checksum
// 		checksum := computeChecksum(versionSecret.Data)

// 		if newAlias.Annotations == nil {
// 			newAlias.Annotations = map[string]string{}
// 		}

// 		newAlias.Annotations["checksum"] = checksum
// 		if versionSecret.Annotations != nil {
// 			newAlias.Annotations["version"] = versionSecret.Annotations["version"]
// 		}

// 		return r.Create(ctx, newAlias)
// 	}

// 	if err != nil {
// 		return err
// 	}

// 	// Aggiorna dati
// 	alias.Data = versionSecret.Data
// 	alias.Type = versionSecret.Type

// 	if alias.Annotations == nil {
// 		alias.Annotations = map[string]string{}
// 	}

// 	// aggiorna metadata
// 	alias.Annotations["checksum"] = computeChecksum(versionSecret.Data)
// 	if versionSecret.Annotations != nil {
// 		alias.Annotations["version"] = versionSecret.Annotations["version"]
// 	}

// 	return r.Update(ctx, alias)
// }

func generationNeeded(gs *securityv1alpha1.GeneratedSecret) (bool, error) {

	// 1. prima creazione
	if gs.Status.CurrentVersion == 0 && gs.Spec.Trigger.OnCreate {
		return true, nil
	}

	// 2. rotazione
	if gs.Spec.Trigger.OnRotate && gs.Spec.RotationInterval != "" {

		interval, _ := time.ParseDuration(gs.Spec.RotationInterval)

		next := gs.Status.LastGeneration.Add(interval)

		if time.Now().After(next) {
			return true, nil
		}
	}

	// 3. spec change
	specHash := hashStructStable(gs.Spec.Parameters)

	if gs.Spec.Trigger.OnSpecChange &&
		gs.Status.ObservedHash != specHash {

		return true, nil
	}

	return false, nil
}

// func computeChecksum(data map[string][]byte) string {

// 	keys := make([]string, 0, len(data))

// 	for k := range data {
// 		keys = append(keys, k)
// 	}

// 	sort.Strings(keys)

// 	h := sha256.New()

// 	for _, k := range keys {
// 		h.Write([]byte(k))
// 		h.Write(data[k])
// 	}

//		return hex.EncodeToString(h.Sum(nil))
//	}
func hashStructStable(obj interface{}) string {

	normalized := normalize(obj)

	bytes, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}

	h := sha256.Sum256(bytes)
	return hex.EncodeToString(h[:])
}

func normalize(v interface{}) interface{} {

	switch val := v.(type) {

	case map[string]interface{}:

		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		ordered := make(map[string]interface{}, len(val))

		for _, k := range keys {
			ordered[k] = normalize(val[k])
		}

		return ordered

	case []interface{}:

		for i := range val {
			val[i] = normalize(val[i])
		}
		return val

	default:
		return val
	}
}

// func computeIdempotencyKey(gs *securityv1alpha1.GeneratedSecret) string {

// 	hashable := struct {
// 		Type       string
// 		Parameters map[string]string
// 		Version    int
// 	}{
// 		Type:       gs.Spec.Type,
// 		Parameters: gs.Spec.Parameters,
// 		Version:    gs.Status.CurrentVersion + 1,
// 	}

// 	return hashStructStable(hashable)
// }

// SetupWithManager sets up the controller with the Manager.
func (r *GeneratedSecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&securityv1alpha1.GeneratedSecret{}).
		Named("generatedsecret").
		Complete(r)
}
