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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

func getContainerImage(containers []corev1.Container, containerName string) string {
	for _, container := range containers {
		if container.Name == containerName {
			return container.Image
		}
	}
	return ""
}

func isReady(pods *corev1.PodList, patroniContainer, desiredImage string) bool {
	readyPods := 0
	for _, pod := range pods.Items {
		if getContainerImage(pod.Spec.Containers, patroniContainer) == desiredImage {
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					readyPods++
				}
			}
		}
	}
	return readyPods == len(pods.Items)
}

func isDegraded(pods *corev1.PodList) bool {
	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			for _, container := range pod.Status.ContainerStatuses {
				if !container.Ready && container.LastTerminationState.Terminated != nil {
					return true
				}
			}
		default:
			// TODO: handle other Phases
			return false
		}
	}
	return false
}

func isScheduled(statefulSet *appsv1.StatefulSet) bool {
	_, ok := statefulSet.Annotations[upgradeScheduleAnnotation]
	return ok
}

func shouldBeUpdated(statefulSet *appsv1.StatefulSet) bool {
	now := time.Now()
	timestamp := statefulSet.Annotations[upgradeScheduleAnnotation]
	scheduledTime, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		// Something went wrong, let's update this StatefulSet
		return true
	}
	return now.After(scheduledTime)
}

func generateParticipantsList(
	currentStatefulSets *appsv1.StatefulSetList,
) (participantsList []string) {
	for _, statefulSet := range currentStatefulSets.Items {
		participantsList = append(participantsList, statefulSet.Annotations[participantAnnotation])
	}
	return
}

func IgnoreAlreadyExists(err error) error {
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}
