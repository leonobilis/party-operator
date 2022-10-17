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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	patroniv1alpha1 "github.com/leonobilis/party-operator/api/v1alpha1"
)

func getCompanionDeploymentDefinition(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
	participantName string,
) *appsv1.Deployment {
	if party.Spec.CompanionDeploymentSpec == nil {
		return &appsv1.Deployment{}
	}
	companionDeploymentSpec := party.Spec.CompanionDeploymentSpec.DeepCopy()

	if companionDeploymentSpec.Template.Labels == nil {
		companionDeploymentSpec.Template.Labels = make(map[string]string)
	}
	companionDeploymentSpec.Template.Labels[partyLabel] = party.Name
	companionDeploymentSpec.Template.Labels[participantLabel] = participantName
	companionDeploymentSpec.Template.Labels[partyRoleLabel] = "companion"

	for i := 0; i < len(companionDeploymentSpec.Template.Spec.Containers); i++ {
		companionDeploymentSpec.Template.Spec.Containers[i].Env = append(
			companionDeploymentSpec.Template.Spec.Containers[i].Env,
			corev1.EnvVar{
				Name:  "MY_PATRONI",
				Value: participantName + "-patroni-svc." + party.Namespace + ".svc",
			},
		)
	}
	if companionDeploymentSpec.Template.Spec.Containers == nil {
		companionDeploymentSpec.Template.Labels = make(map[string]string)
	}

	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        participantName + "-companion",
			Namespace:   party.Namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: *companionDeploymentSpec,
	}

	deployment.Labels[partyLabel] = party.Name
	deployment.Labels[participantLabel] = participantName

	return &deployment
}

func getPatroniStatefulSetDefinition(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
	participantName string,
) *appsv1.StatefulSet {
	statefulSetSpec := party.Spec.PatroniStatefulSetSpec.DeepCopy()
	for i := 0; i < len(statefulSetSpec.Template.Spec.Containers); i++ {
		if statefulSetSpec.Template.Spec.Containers[i].Name == party.Spec.PatroniContainerName {
			statefulSetSpec.Template.Spec.Containers[i].Image = party.Spec.Images[0]
			break
		}
	}
	if statefulSetSpec.Template.Labels == nil {
		statefulSetSpec.Template.Labels = make(map[string]string)
	}
	statefulSetSpec.Template.Labels[partyLabel] = party.Name
	statefulSetSpec.Template.Labels[participantLabel] = participantName
	statefulSetSpec.Template.Labels[partyRoleLabel] = "patroni"
	for i := 0; i < len(statefulSetSpec.VolumeClaimTemplates); i++ {
		if statefulSetSpec.VolumeClaimTemplates[i].Labels == nil {
			statefulSetSpec.VolumeClaimTemplates[i].Labels = make(map[string]string)
		}
		statefulSetSpec.VolumeClaimTemplates[i].Labels[partyLabel] = party.Name
		statefulSetSpec.VolumeClaimTemplates[i].Labels[participantLabel] = participantName
	}
	statefulSetSpec.Template.Spec.ServiceAccountName = serviceAccountPrefix + party.Name

	statefulSet := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        participantName + "-patroni",
			Namespace:   party.Namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: *statefulSetSpec,
	}

	statefulSet.Labels[partyLabel] = party.Name
	statefulSet.Labels[participantLabel] = participantName
	statefulSet.Annotations[partyAnnotation] = party.Name
	statefulSet.Annotations[participantAnnotation] = participantName
	statefulSet.Annotations[currentImageAnnotation] = "0"

	return &statefulSet
}

func getServiceDefinition(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
	participantName string,
) *corev1.Service {
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        participantName + "-patroni-svc",
			Namespace:   party.Namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "postgresql",
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: make(map[string]string),
		},
	}

	service.Labels[partyLabel] = party.Name
	service.Labels[participantLabel] = participantName
	service.Spec.Selector[participantLabel] = participantName
	service.Spec.Selector["role"] = "master"

	return &service
}

func getServiceAccountDefinition(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
) *corev1.ServiceAccount {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceAccountPrefix + party.Name,
			Namespace:   party.Namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
	}

	sa.Labels[partyLabel] = party.Name

	return &sa
}

func getRoleDefinition(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
) *rbacv1.Role {
	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:        rolePrefix + party.Name,
			Namespace:   party.Namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"create", "get", "list", "patch", "update", "watch", "delete"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"create", "patch", "get", "list"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"patch", "get", "list", "update", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"nodes"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}
	role.Labels[partyLabel] = party.Name

	return &role
}

func getRoleBindingDefinition(
	ctx context.Context,
	party *patroniv1alpha1.PatroniParty,
) *rbacv1.RoleBinding {
	role := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        roleBindingPrefix + party.Name,
			Namespace:   party.Namespace,
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: "ServiceAccount",
				Name: serviceAccountPrefix + party.Name,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.SchemeGroupVersion.Group,
			Kind:     "Role",
			Name:     rolePrefix + party.Name,
		},
	}
	role.Labels[partyLabel] = party.Name

	return &role
}
