/*
Copyright 2022.

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

package controllers

import (
	"context"
	"strconv"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	patroniv1alpha1 "github.com/leonobilis/party-operator/api/v1alpha1"
	"github.com/leonobilis/party-operator/helpers"
)

const (
	ssOwnerKey       = ".metadata.controller"
	partyLabel       = "party"
	participantLabel = "party-participant"
	partyRoleLabel   = "party-role"

	serviceAccountPrefix = "patroni-sa-"
	rolePrefix           = "patroni-role-"
	roleBindingPrefix    = "patroni-rb-"
)

var (
	apiGVStr                  = patroniv1alpha1.GroupVersion.String()
	apiGroup                  = patroniv1alpha1.GroupVersion.Group
	partyAnnotation           = apiGroup + "/" + partyLabel
	participantAnnotation     = apiGroup + "/" + participantLabel
	currentImageAnnotation    = apiGroup + "/current-image"
	upgradeScheduleAnnotation = apiGroup + "/upgrade-schedule"
	finalizerName             = apiGroup + "/janitor"
)

// PatroniPartyReconciler reconciles a PatroniParty object
type PatroniPartyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=patroni.party.operator,resources=patroniparties,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=patroni.party.operator,resources=patroniparties/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=patroni.party.operator,resources=patroniparties/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;patch;update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;delete;deletecollection
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=create;get;list;patch;update;watch;delete
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete

func (r *PatroniPartyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.V(1).Info("Request", "Name", req.Name, "Namespace", req.Namespace)

	var party patroniv1alpha1.PatroniParty
	if err := r.Get(ctx, req.NamespacedName, &party); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "unable to fetch PatroniParty resource")
		return ctrl.Result{}, err
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if party.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(&party, finalizerName) {
			controllerutil.AddFinalizer(&party, finalizerName)
			if err := r.Update(ctx, &party); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.createCommonResources(ctx, &party); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(&party, finalizerName) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(ctx, &party); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(&party, finalizerName)
			if err := r.Update(ctx, &party); err != nil {
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	var childStatefulSets appsv1.StatefulSetList
	if err := r.List(ctx, &childStatefulSets, client.InNamespace(req.Namespace), client.MatchingFields{ssOwnerKey: req.Name}); err != nil {
		log.Error(err, "unable to list child StatefulSets")
		return ctrl.Result{}, err
	}

	participantsDiff := party.Spec.Participants - len(childStatefulSets.Items)

	if participantsDiff > 0 {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.createParticipants(ctx, &party, participantsDiff, &childStatefulSets)
	} else if participantsDiff < 0 {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, r.deleteParticipants(ctx, &party, -participantsDiff, &childStatefulSets)
	}

	party.Status.ParticipantsList = make([]string, 0)
	party.Status.RunningList = make([]string, 0)
	party.Status.DegradedList = make([]string, 0)
	toSchedule := make([]*appsv1.StatefulSet, 0)
	toUpdate := make([]*appsv1.StatefulSet, 0)
	for _, statefulSet := range childStatefulSets.Items {
		participantName := statefulSet.Annotations[participantAnnotation]
		party.Status.ParticipantsList = append(party.Status.ParticipantsList, participantName)

		var pods corev1.PodList
		if err := r.List(ctx, &pods, client.InNamespace(req.Namespace), client.MatchingLabels{
			partyLabel:       party.Name,
			participantLabel: participantName,
			partyRoleLabel:   "patroni",
		}); err != nil {
			log.Error(err, "unable to list Pods of Patroni StatefulSets")
			return ctrl.Result{}, err
		}

		if isReady(&pods, party.Spec.PatroniContainerName, getContainerImage(statefulSet.Spec.Template.Spec.Containers, party.Spec.PatroniContainerName)) {
			if !isScheduled(&statefulSet) {
				toSchedule = append(toSchedule, &statefulSet)
			} else if shouldBeUpdated(&statefulSet) {
				toUpdate = append(toUpdate, &statefulSet)
			}
			party.Status.RunningList = append(party.Status.RunningList, participantName)
		} else if isDegraded(&pods) {
			party.Status.DegradedList = append(party.Status.DegradedList, participantName)
		} else {
			party.Status.RunningList = append(party.Status.RunningList, participantName)
		}
	}

	party.Status.Participants = len(party.Status.ParticipantsList)
	party.Status.Running = len(party.Status.RunningList)
	party.Status.Degraded = len(party.Status.DegradedList)

	log.V(1).Info("Participants report",
		"Participants", party.Status.Participants,
		"Running", party.Status.Running,
		"Degraded", party.Status.Degraded,
		"To Schedule", len(toSchedule),
		"To Update", len(toUpdate),
	)

	if err := r.updateStatus(ctx, &party); err != nil {
		return ctrl.Result{}, err
	}

	scheduleTime := time.Now().Add(time.Duration(party.Spec.UpgradeDelay) * time.Second)
	for _, statefulSet := range toSchedule {
		statefulSet.Annotations[upgradeScheduleAnnotation] = scheduleTime.Format(time.RFC3339Nano)
		if err := r.Update(ctx, statefulSet); err != nil {
			log.Error(err, "unable to update Patroni StatefulSet", "StatefulSet", statefulSet.Name)
			return ctrl.Result{}, err
		}
	}

	for _, statefulSet := range toUpdate {
		delete(statefulSet.Annotations, upgradeScheduleAnnotation)
		nextImage := 0
		if currentImage, err := strconv.Atoi(statefulSet.Annotations[currentImageAnnotation]); err == nil && currentImage < len(party.Spec.Images) {
			nextImage = (currentImage + 1) % len(party.Spec.Images)
		}
		statefulSet.Annotations[currentImageAnnotation] = strconv.Itoa(nextImage)
		for i := 0; i < len(statefulSet.Spec.Template.Spec.Containers); i++ {
			if statefulSet.Spec.Template.Spec.Containers[i].Name == party.Spec.PatroniContainerName {
				statefulSet.Spec.Template.Spec.Containers[i].Image = party.Spec.Images[nextImage]
				break
			}
		}

		if err := r.Update(ctx, statefulSet); err != nil {
			log.Error(err, "unable to update Patroni StatefulSet", "StatefulSet", statefulSet.Name)
			return ctrl.Result{}, err
		}
	}

	if len(toSchedule) > 0 {
		return ctrl.Result{RequeueAfter: time.Duration(party.Spec.UpgradeDelay) * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

func (r *PatroniPartyReconciler) updateStatus(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
) error {
	err := r.Status().Update(ctx, party)
	if err != nil {
		log.FromContext(ctx).Error(err, "unable to update status of PatroniParty resource")
	}
	time.Sleep(time.Millisecond) // Status updates throttling
	return err
}

func (r *PatroniPartyReconciler) createCommonResources(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
) error {
	log := log.FromContext(ctx)
	log.Info("Creating ServiceAccount", "ServiceAccount", serviceAccountPrefix+party.Name)
	if err := r.Create(ctx, getServiceAccountDefinition(ctx, party)); IgnoreAlreadyExists(err) != nil {
		log.Error(err, "unable to create ServiceAccount")
		return err
	}
	log.Info("Creating Role", "Role", rolePrefix+party.Name)
	if err := r.Create(ctx, getRoleDefinition(ctx, party)); IgnoreAlreadyExists(err) != nil {
		log.Error(err, "unable to create Role")
		return err
	}
	log.Info("Creating RoleBinding", "RoleBinding", roleBindingPrefix+party.Name)
	if err := r.Create(ctx, getRoleBindingDefinition(ctx, party)); IgnoreAlreadyExists(err) != nil {
		log.Error(err, "unable to create RoleBinding")
		return err
	}
	return nil
}

func (r *PatroniPartyReconciler) createParticipants(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
	diff int,
	currentStatefulSets *appsv1.StatefulSetList,
) error {
	log := log.FromContext(ctx)
	log.Info("Creating participants", "count", diff)
	for i := 0; i < diff; i++ {
		participantName := helpers.GenerateName(generateParticipantsList(currentStatefulSets))
		statefulSet := getPatroniStatefulSetDefinition(ctx, party, participantName)
		if err := ctrl.SetControllerReference(party, statefulSet, r.Scheme); err != nil {
			log.Error(err, "unable to construct StatefulSet from template")
			return err
		}
		if err := r.Create(ctx, statefulSet); err != nil {
			log.Error(err, "unable to create StatefulSet", "StatefulSet", statefulSet)
			return err
		}
		service := getServiceDefinition(ctx, party, participantName)
		if err := r.Create(ctx, service); err != nil {
			log.Error(err, "unable to create Service", "Service", service)
			return err
		}
		if party.Spec.CompanionDeploymentSpec != nil {
			if err := r.Create(ctx, getCompanionDeploymentDefinition(ctx, party, participantName)); err != nil {
				log.Error(err, "unable to create Companion Deployment")
				return err
			}
		}
	}
	return nil
}

func (r *PatroniPartyReconciler) deleteParticipants(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
	diff int,
	currentStatefulSets *appsv1.StatefulSetList,
) error {
	log := log.FromContext(ctx)
	log.Info("Deleting participants", "count", diff)
	currentLen := len(currentStatefulSets.Items)
	for i := 0; i < diff; i++ {
		statefulSet := currentStatefulSets.Items[currentLen-i-1]
		if err := r.Delete(ctx, &statefulSet); client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to delete StatefulSet", "StatefulSet", statefulSet)
			return err
		}
		participant := statefulSet.Annotations[participantAnnotation]
		inNamespaceOption := client.InNamespace(party.Namespace)
		matchingLabelsOption := client.MatchingLabels{partyLabel: party.Name, participantLabel: participant}
		if err := r.DeleteAllOf(ctx, &appsv1.Deployment{}, inNamespaceOption, matchingLabelsOption); client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to delete Deployments", "participant", participant)
			return err
		}
		var services corev1.ServiceList
		if err := r.List(ctx, &services, inNamespaceOption, matchingLabelsOption); client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to list Services", "participant", participant)
			return err
		}
		for _, service := range services.Items {
			if err := r.Delete(ctx, &service); client.IgnoreNotFound(err) != nil {
				log.Error(err, "unable to delete Service", "Service", service)
				return err
			}
		}
		if err := r.DeleteAllOf(ctx, &corev1.PersistentVolumeClaim{}, inNamespaceOption, matchingLabelsOption); client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to delete PersistentVolumeClaims", "participant", participant)
			return err
		}
	}
	return nil
}

func (r *PatroniPartyReconciler) deleteExternalResources(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
) error {
	log := log.FromContext(ctx)
	log.Info("Deleting resources in finalizer")
	inNamespaceOption := client.InNamespace(party.Namespace)
	matchingLabelsOption := client.MatchingLabels{partyLabel: party.Name}
	if err := r.DeleteAllOf(ctx, &appsv1.Deployment{}, inNamespaceOption, matchingLabelsOption); client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to delete Deployments", "party", party.Name)
		return err
	}
	var services corev1.ServiceList
	if err := r.List(ctx, &services, inNamespaceOption, matchingLabelsOption); client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to list Services", "party", party.Name)
		return err
	}
	for _, service := range services.Items {
		if err := r.Delete(ctx, &service); client.IgnoreNotFound(err) != nil {
			log.Error(err, "unable to delete Service", "Service", service)
			return err
		}
	}
	if err := r.DeleteAllOf(ctx, &corev1.PersistentVolumeClaim{}, inNamespaceOption, matchingLabelsOption); client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to delete PersistentVolumeClaims", "party", party.Name)
		return err
	}
	rb := rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: roleBindingPrefix + party.Name, Namespace: party.Namespace}}
	if err := r.Delete(ctx, &rb); client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to delete Service", "RoleBinding", rb)
		return err
	}
	role := rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: rolePrefix + party.Name, Namespace: party.Namespace}}
	if err := r.Delete(ctx, &role); client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to delete Role", "Role", role)
		return err
	}
	sa := corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: serviceAccountPrefix + party.Name, Namespace: party.Namespace}}
	if err := r.Delete(ctx, &sa); client.IgnoreNotFound(err) != nil {
		log.Error(err, "unable to delete ServiceAccount", "ServiceAccount", sa)
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PatroniPartyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &appsv1.StatefulSet{}, ssOwnerKey, func(rawObj client.Object) []string {
		sts := rawObj.(*appsv1.StatefulSet)
		owner := metav1.GetControllerOf(sts)
		if owner == nil {
			return nil
		}
		if owner.APIVersion != apiGVStr || owner.Kind != "PatroniParty" {
			return nil
		}
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&patroniv1alpha1.PatroniParty{}).
		Owns(&appsv1.StatefulSet{}).
		// Owns(&appsv1.Deployment{}).
		WithEventFilter(PatroniPartyPredicate{}).
		// WithOptions(controller.Options{MaxConcurrentReconciles: 3}).
		Complete(r)
}
