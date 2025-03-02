//
// Copyright 2021 Red Hat, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	devfileAPIV1 "github.com/devfile/api/v2/pkg/apis/workspaces/v1alpha2"
	"github.com/devfile/api/v2/pkg/attributes"
	"github.com/devfile/api/v2/pkg/devfile"
	v2 "github.com/devfile/library/pkg/devfile/parser/data/v2"
	"github.com/devfile/library/pkg/devfile/parser/data/v2/common"
	appstudiov1alpha1 "github.com/redhat-appstudio/application-service/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"
)

func TestUpdateApplicationDevfileModel(t *testing.T) {
	tests := []struct {
		name           string
		projects       []devfileAPIV1.Project
		attributes     attributes.Attributes
		containerImage string
		component      appstudiov1alpha1.Component
		wantErr        bool
	}{
		{
			name: "Project already present",
			projects: []devfileAPIV1.Project{
				{
					Name: "duplicate",
				},
			},
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "duplicate",
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Project added successfully",
			projects: []devfileAPIV1.Project{
				{
					Name: "present",
				},
			},
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "new",
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{
								URL: "url",
							},
						},
					},
				},
			},
		},
		{
			name: "Git source in Component is nil",
			projects: []devfileAPIV1.Project{
				{
					Name: "present",
				},
			},
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "new",
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: nil,
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name:     "Devfile Projects list is nil",
			projects: nil,
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "new",
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: nil,
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name:       "Container image added successfully",
			attributes: attributes.Attributes{}.PutString("containerImage/otherComponent", "other-image"),
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName:  "new",
					ContainerImage: "an-image",
				},
			},
		},
		{
			name:       "Container image already exists",
			attributes: attributes.Attributes{}.PutString("containerImage/new", "an-image"),
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName:  "new",
					ContainerImage: "an-image",
				},
			},
			wantErr: true,
		},
		{
			name:       "Container image already exists, but invalid entry",
			attributes: attributes.Attributes{}.Put("containerImage/new", make(chan error), nil),
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName:  "new",
					ContainerImage: "an-image",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devfileData := &v2.DevfileV2{
				Devfile: devfileAPIV1.Devfile{
					DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
						DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
							Attributes: tt.attributes,
							Projects:   tt.projects,
						},
					},
				},
			}
			r := ComponentReconciler{}
			err := r.updateApplicationDevfileModel(devfileData, tt.component)
			if tt.wantErr && (err == nil) {
				t.Error("wanted error but got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("got unexpected error %v", err)
			} else if err == nil {
				if tt.component.Spec.Source.GitSource != nil {
					projects, err := devfileData.GetProjects(common.DevfileOptions{})
					if err != nil {
						t.Errorf("got unexpected error: %v", err)
					}
					matched := false
					for _, project := range projects {
						projectGitSrc := project.ProjectSource.Git
						if project.Name == tt.component.Spec.ComponentName && projectGitSrc != nil && projectGitSrc.Remotes["origin"] == tt.component.Spec.Source.GitSource.URL {
							matched = true
						}
					}

					if !matched {
						t.Errorf("unable to find devfile with project: %s", tt.component.Spec.ComponentName)
					}

				} else {
					devfileAttr, err := devfileData.GetAttributes()
					if err != nil {
						t.Errorf("got unexpected error: %v", err)
					}
					if devfileAttr == nil {
						t.Errorf("devfile attributes should not be nil")
					}
					containerImage := devfileAttr.GetString("containerImage/new", &err)
					if err != nil {
						t.Errorf("got unexpected error: %v", err)
					}
					if containerImage != tt.component.Spec.ContainerImage {
						t.Errorf("unable to find component with container iamge: %s", tt.component.Spec.ContainerImage)
					}
				}
			}
		})
	}
}

func TestUpdateComponentDevfileModel(t *testing.T) {

	storage1GiResource, err := resource.ParseQuantity("1Gi")
	if err != nil {
		t.Error(err)
	}
	core500mResource, err := resource.ParseQuantity("500m")
	if err != nil {
		t.Error(err)
	}

	originalResources := corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:     core500mResource,
			corev1.ResourceMemory:  storage1GiResource,
			corev1.ResourceStorage: storage1GiResource,
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:     core500mResource,
			corev1.ResourceMemory:  storage1GiResource,
			corev1.ResourceStorage: storage1GiResource,
		},
	}

	envAttributes := attributes.Attributes{}.FromMap(map[string]interface{}{containerENVKey: []corev1.EnvVar{{Name: "FOO", Value: "foo"}}}, &err)
	if err != nil {
		t.Error(err)
	}

	env := []corev1.EnvVar{
		{
			Name:  "FOO",
			Value: "foo1",
		},
		{
			Name:  "BAR",
			Value: "bar1",
		},
	}

	tests := []struct {
		name           string
		components     []devfileAPIV1.Component
		component      appstudiov1alpha1.Component
		updateExpected bool
		wantErr        bool
	}{
		{
			name: "No kubernetes component",
			components: []devfileAPIV1.Component{
				{
					Name: "component1",
					ComponentUnion: devfileAPIV1.ComponentUnion{
						Container: &devfileAPIV1.ContainerComponent{},
					},
				},
			},
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "componentName",
				},
			},
		},
		{
			name: "one kubernetes component",
			components: []devfileAPIV1.Component{
				{
					Name:       "component1",
					Attributes: envAttributes.PutInteger(containerImagePortKey, 1001),
					ComponentUnion: devfileAPIV1.ComponentUnion{
						Kubernetes: &devfileAPIV1.KubernetesComponent{},
					},
				},
			},
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "componentName",
					Application:   "applicationName",
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{
								URL: "url",
							},
						},
					},
					Route:      "route1",
					Replicas:   1,
					TargetPort: 1111,
					Env:        env,
					Resources:  originalResources,
				},
			},
			updateExpected: true,
		},
		{
			name: "two kubernetes components",
			components: []devfileAPIV1.Component{
				{
					Name:       "component1",
					Attributes: envAttributes.PutInteger(containerImagePortKey, 1001),
					ComponentUnion: devfileAPIV1.ComponentUnion{
						Kubernetes: &devfileAPIV1.KubernetesComponent{},
					},
				},
				{
					Name:       "component2",
					Attributes: envAttributes.PutInteger(containerImagePortKey, 3333).PutString(memoryLimitKey, "2Gi"),
					ComponentUnion: devfileAPIV1.ComponentUnion{
						Kubernetes: &devfileAPIV1.KubernetesComponent{},
					},
				},
			},
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "componentName",
					Application:   "applicationName",
					Source: appstudiov1alpha1.ComponentSource{
						ComponentSourceUnion: appstudiov1alpha1.ComponentSourceUnion{
							GitSource: &appstudiov1alpha1.GitSource{
								URL: "url",
							},
						},
					},
					Route:      "route1",
					Replicas:   1,
					TargetPort: 1111,
					Env:        env,
					Resources:  originalResources,
				},
			},
			updateExpected: true,
		},
		{
			name: "Component with envFrom component - should error out as it's not supported right now",
			components: []devfileAPIV1.Component{
				{
					Name:       "component1",
					Attributes: envAttributes,
					ComponentUnion: devfileAPIV1.ComponentUnion{
						Kubernetes: &devfileAPIV1.KubernetesComponent{},
					},
				},
			},
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "component1",
					Env: []corev1.EnvVar{
						{
							Name:  "FOO",
							Value: "foo",
						},
						{
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									Key: "test",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Component with invalid component type - should error out",
			components: []devfileAPIV1.Component{
				{
					Name:           "component1",
					ComponentUnion: devfileAPIV1.ComponentUnion{},
				},
			},
			component: appstudiov1alpha1.Component{
				Spec: appstudiov1alpha1.ComponentSpec{
					ComponentName: "component1",
					Env: []corev1.EnvVar{
						{
							Name:  "FOO",
							Value: "foo",
						},
						{
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									Key: "test",
								},
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			devfileData := &v2.DevfileV2{
				Devfile: devfileAPIV1.Devfile{
					DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
						DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
							Components: tt.components,
						},
					},
				},
			}

			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{
				Development: true,
			})))
			r := ComponentReconciler{
				Log: ctrl.Log.WithName("TestUpdateComponentDevfileModel"),
			}
			err := r.updateComponentDevfileModel(ctrl.Request{}, devfileData, tt.component)
			if tt.wantErr && (err == nil) {
				t.Error("wanted error but got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("got unexpected error %v", err)
			} else if err == nil {
				if tt.updateExpected {
					// it has been updated
					checklist := updateChecklist{
						route:     tt.component.Spec.Route,
						replica:   tt.component.Spec.Replicas,
						port:      tt.component.Spec.TargetPort,
						env:       tt.component.Spec.Env,
						resources: tt.component.Spec.Resources,
					}

					verifyHASComponentUpdates(devfileData, checklist, t)
				}
			}
		})
	}
}

func TestUpdateComponentStub(t *testing.T) {
	var err error
	envAttributes := attributes.Attributes{}.FromMap(map[string]interface{}{containerENVKey: []corev1.EnvVar{{Name: "name1", Value: "value1"}}}, &err)
	if err != nil {
		t.Error(err)
	}

	componentsValid := []devfileAPIV1.Component{
		{
			Name: "component1",
			Attributes: envAttributes.PutInteger(replicaKey, 1).PutString(routeKey, "route1").PutInteger(
				containerImagePortKey, 1001).PutString(cpuLimitKey, "2").PutString(cpuRequestKey, "700m").PutString(
				memoryLimitKey, "500Mi").PutString(memoryRequestKey, "400Mi").PutString(
				storageLimitKey, "400Mi").PutString(storageRequestKey, "200Mi"),
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{
					K8sLikeComponent: devfileAPIV1.K8sLikeComponent{
						K8sLikeComponentLocation: devfileAPIV1.K8sLikeComponentLocation{
							Uri: "testLocation",
						},
					},
				},
			},
		},
		{
			Name:       "component2",
			Attributes: attributes.Attributes{}.PutInteger(containerImagePortKey, 1003),
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{
					K8sLikeComponent: devfileAPIV1.K8sLikeComponent{
						K8sLikeComponentLocation: devfileAPIV1.K8sLikeComponentLocation{
							Uri: "testLocation",
						},
					},
				},
			},
		},
	}

	componentsReplicaErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.PutBoolean(replicaKey, true),
		},
	}

	componentsContainerPortErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.PutBoolean(containerImagePortKey, true),
		},
	}

	componentsRouteErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.Put(routeKey, []string{"a", "b"}, &err),
		},
	}
	if err != nil {
		t.Errorf("unexpected err: %+v", err)
		return
	}

	componentsStorageLimitErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.Put(storageLimitKey, []string{"a", "b"}, &err),
		},
	}
	if err != nil {
		t.Errorf("unexpected err: %+v", err)
		return
	}

	componentsStorageRequestErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.Put(storageRequestKey, []string{"a", "b"}, &err),
		},
	}
	if err != nil {
		t.Errorf("unexpected err: %+v", err)
		return
	}

	componentsCpuLimitErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.Put(cpuLimitKey, []string{"a", "b"}, &err),
		},
	}
	if err != nil {
		t.Errorf("unexpected err: %+v", err)
		return
	}

	componentsCpuRequestErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.Put(cpuRequestKey, []string{"a", "b"}, &err),
		},
	}
	if err != nil {
		t.Errorf("unexpected err: %+v", err)
		return
	}

	componentsMemoryLimitErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.Put(memoryLimitKey, []string{"a", "b"}, &err),
		},
	}
	if err != nil {
		t.Errorf("unexpected err: %+v", err)
		return
	}

	componentsMemoryRequestErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.Put(memoryRequestKey, []string{"a", "b"}, &err),
		},
	}
	if err != nil {
		t.Errorf("unexpected err: %+v", err)
		return
	}

	componentsCpuLimitParseErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.PutString(cpuLimitKey, "xyz"),
		},
	}

	componentsMemoryLimitParseErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.PutString(memoryLimitKey, "xyz"),
		},
	}

	componentsStorageLimitParseErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.PutString(storageLimitKey, "xyz"),
		},
	}

	componentsCpuRequestParseErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.PutString(cpuRequestKey, "xyz"),
		},
	}

	componentsMemoryRequestParseErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.PutString(memoryRequestKey, "xyz"),
		},
	}

	componentsStorageRequestParseErr := []devfileAPIV1.Component{
		{
			Name: "component1",
			ComponentUnion: devfileAPIV1.ComponentUnion{
				Kubernetes: &devfileAPIV1.KubernetesComponent{},
			},
			Attributes: attributes.Attributes{}.PutString(storageRequestKey, "xyz"),
		},
	}

	tests := []struct {
		name             string
		devfilesDataMap  map[string]*v2.DevfileV2
		devfilesURLMap   map[string]string
		dockerfileURLMap map[string]string
		isNil            bool
		wantErr          bool
	}{
		{
			name: "Kubernetes Components present",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsValid,
							},
						},
					},
				},
			},
		},
		{
			name: "Kubernetes Components present with a devfile URL",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsValid,
							},
						},
					},
				},
			},
			devfilesURLMap: map[string]string{
				"./": "http://somelink",
			},
		},
		{
			name: "Kubernetes Components present with a devfile & dockerfile URL",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsValid,
							},
						},
					},
				},
			},
			devfilesURLMap: map[string]string{
				"./": "http://somelink",
			},
			dockerfileURLMap: map[string]string{
				"./": "http://someotherlink",
			},
		},
		{
			name: "dockerfile URL only",
			dockerfileURLMap: map[string]string{
				"./": "http://someotherlink",
			},
		},
		{
			name: "No Kubernetes Components present",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{},
						},
					},
				},
			},
		},
		{
			name: "Check err condition",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{},
						},
					},
				},
			},
			isNil:   true,
			wantErr: true,
		},
		{
			name: "Check err for replica as non integer",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsReplicaErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for container port as non integer",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsContainerPortErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for route as non string",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsRouteErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for storage limit as non string",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsStorageLimitErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for storage request as non string",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsStorageRequestErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for cpu limit as non string",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsCpuLimitErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for cpu request as non string",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsCpuRequestErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for memory limit as non string",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsMemoryLimitErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for memory request as non string",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsMemoryRequestErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for cpu limit parse err",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsCpuLimitParseErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for memory limit parse err",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsMemoryLimitParseErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for storage limit parse err",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsStorageLimitParseErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for cpu request parse err",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsCpuRequestParseErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for memory request parse err",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsMemoryRequestParseErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Check err for storage request parse err",
			devfilesDataMap: map[string]*v2.DevfileV2{
				"./": {
					Devfile: devfileAPIV1.Devfile{
						DevfileHeader: devfile.DevfileHeader{
							SchemaVersion: "2.1.0",
							Metadata: devfile.DevfileMetadata{
								Name:        "test-devfile",
								Language:    "language",
								ProjectType: "project",
							},
						},
						DevWorkspaceTemplateSpec: devfileAPIV1.DevWorkspaceTemplateSpec{
							DevWorkspaceTemplateSpecContent: devfileAPIV1.DevWorkspaceTemplateSpecContent{
								Components: componentsStorageRequestParseErr,
							},
						},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			componentDetectionQuery := appstudiov1alpha1.ComponentDetectionQuery{
				Spec: appstudiov1alpha1.ComponentDetectionQuerySpec{
					GitSource: appstudiov1alpha1.GitSource{
						URL: "url",
					},
				},
			}
			devfilesMap := make(map[string][]byte)

			for context, devfileData := range tt.devfilesDataMap {
				yamlData, err := yaml.Marshal(devfileData)
				if err != nil {
					t.Errorf("unexpected error %v", err)
				}
				devfilesMap[context] = yamlData
			}

			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zap.Options{
				Development: true,
			})))
			fakeClient := NewFakeClient(t)
			fakeClient.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
				return nil
			}
			r := ComponentDetectionQueryReconciler{
				Client: fakeClient,
				Log:    ctrl.Log.WithName("TestUpdateComponentStub"),
			}
			var err error
			if tt.isNil {
				err = r.updateComponentStub(ctrl.Request{}, nil, nil, devfilesMap, nil, nil)
			} else {
				err = r.updateComponentStub(ctrl.Request{}, nil, &componentDetectionQuery, devfilesMap, tt.devfilesURLMap, tt.dockerfileURLMap)
			}

			if tt.wantErr && (err == nil) {
				t.Error("wanted error but got nil")
			} else if !tt.wantErr && err != nil {
				t.Errorf("got unexpected error %v", err)
			} else if err == nil {
				for _, hasCompDetection := range componentDetectionQuery.Status.ComponentDetected {
					// Application Name
					assert.Equal(t, hasCompDetection.ComponentStub.Application, "insert-application-name", "The application name should match the generic name")

					if len(tt.devfilesDataMap) != 0 {
						// Language
						assert.Equal(t, hasCompDetection.Language, tt.devfilesDataMap[hasCompDetection.ComponentStub.Source.GitSource.Context].Metadata.Language, "The language should be the same")

						// Project Type
						assert.Equal(t, hasCompDetection.ProjectType, tt.devfilesDataMap[hasCompDetection.ComponentStub.Source.GitSource.Context].Metadata.ProjectType, "The project type should be the same")

						// Devfile Found
						assert.Equal(t, hasCompDetection.DevfileFound, len(tt.devfilesURLMap[hasCompDetection.ComponentStub.Source.GitSource.Context]) == 0, "The devfile found did not match expected")

						// Component Name
						assert.Contains(t, hasCompDetection.ComponentStub.ComponentName, "url", "The component name did not match the expected")

						// Devfile URL
						if len(tt.devfilesURLMap) > 0 {
							assert.NotNil(t, hasCompDetection.ComponentStub.Source.GitSource, "The git source cannot be nil for this test")
							assert.Equal(t, hasCompDetection.ComponentStub.Source.GitSource.URL, "url", "The URL should match")
							assert.Equal(t, hasCompDetection.ComponentStub.Source.GitSource.DevfileURL, tt.devfilesURLMap[hasCompDetection.ComponentStub.Source.GitSource.Context], "The devfile URL should match")
						}

						// Dockerfile URL
						if len(tt.dockerfileURLMap) > 0 {
							assert.NotNil(t, hasCompDetection.ComponentStub.Source.GitSource, "The git source cannot be nil for this test")
							assert.Equal(t, hasCompDetection.ComponentStub.Source.GitSource.URL, "url", "The URL should match")
							assert.Equal(t, hasCompDetection.ComponentStub.Source.GitSource.DockerfileURL, tt.dockerfileURLMap[hasCompDetection.ComponentStub.Source.GitSource.Context], "The dockerfile URL should match")
						}

						for _, devfileComponent := range tt.devfilesDataMap[hasCompDetection.ComponentStub.Source.GitSource.Context].Components {
							if devfileComponent.Kubernetes != nil {
								componentAttributes := devfileComponent.Attributes
								var containerENVs []corev1.EnvVar
								err := componentAttributes.GetInto(containerENVKey, &containerENVs)
								assert.Nil(t, err, "err should be nil")
								for _, devfileEnv := range containerENVs {
									matched := false
									for _, compEnv := range hasCompDetection.ComponentStub.Env {
										if devfileEnv.Name == compEnv.Name && devfileEnv.Value == compEnv.Value {
											matched = true
										}
									}
									assert.True(t, matched, "env %s:%s should match", devfileEnv.Name, devfileEnv.Value)
								}

								limits := hasCompDetection.ComponentStub.Resources.Limits
								if len(limits) > 0 {
									resourceCPULimit := limits[corev1.ResourceCPU]
									assert.Equal(t, resourceCPULimit.String(), devfileComponent.Attributes.GetString(cpuLimitKey, &err), "The cpu limit should be the same")
									assert.Nil(t, err, "err should be nil")

									resourceMemoryLimit := limits[corev1.ResourceMemory]
									assert.Equal(t, resourceMemoryLimit.String(), devfileComponent.Attributes.GetString(memoryLimitKey, &err), "The memory limit should be the same")
									assert.Nil(t, err, "err should be nil")

									resourceStorageLimit := limits[corev1.ResourceStorage]
									assert.Equal(t, resourceStorageLimit.String(), devfileComponent.Attributes.GetString(storageLimitKey, &err), "The storage limit should be the same")
									assert.Nil(t, err, "err should be nil")
								}

								requests := hasCompDetection.ComponentStub.Resources.Requests
								if len(requests) > 0 {
									resourceCPURequest := requests[corev1.ResourceCPU]
									assert.Equal(t, resourceCPURequest.String(), devfileComponent.Attributes.GetString(cpuRequestKey, &err), "The cpu request should be the same")
									assert.Nil(t, err, "err should be nil")

									resourceMemoryRequest := requests[corev1.ResourceMemory]
									assert.Equal(t, resourceMemoryRequest.String(), devfileComponent.Attributes.GetString(memoryRequestKey, &err), "The memory request should be the same")
									assert.Nil(t, err, "err should be nil")

									resourceStorageRequest := requests[corev1.ResourceStorage]
									assert.Equal(t, resourceStorageRequest.String(), devfileComponent.Attributes.GetString(storageRequestKey, &err), "The storage request should be the same")
									assert.Nil(t, err, "err should be nil")
								}

								assert.Equal(t, hasCompDetection.ComponentStub.Replicas, int(devfileComponent.Attributes.GetNumber(replicaKey, &err)), "The replicas should be the same")
								assert.Nil(t, err, "err should be nil")

								assert.Equal(t, hasCompDetection.ComponentStub.TargetPort, int(devfileComponent.Attributes.GetNumber(containerImagePortKey, &err)), "The target port should be the same")
								assert.Nil(t, err, "err should be nil")

								assert.Equal(t, hasCompDetection.ComponentStub.Route, devfileComponent.Attributes.GetString(routeKey, &err), "The route should be the same")
								assert.Nil(t, err, "err should be nil")

								break // dont check for the second Kubernetes component
							}
						}
					}

					if len(tt.dockerfileURLMap) != 0 {
						// Language
						assert.Equal(t, hasCompDetection.Language, "Dockerfile", "The language should be the same")

						// Project Type
						assert.Equal(t, hasCompDetection.ProjectType, "Dockerfile", "The project type should be the same")

						// Devfile Found
						assert.Equal(t, hasCompDetection.DevfileFound, false, "The devfile found did not match expected")

						// Component Name
						assert.Contains(t, hasCompDetection.ComponentStub.ComponentName, "url", "The component name did not match the expected")

						// Dockerfile URL
						if len(tt.dockerfileURLMap) > 0 {
							assert.NotNil(t, hasCompDetection.ComponentStub.Source.GitSource, "The git source cannot be nil for this test")
							assert.Equal(t, hasCompDetection.ComponentStub.Source.GitSource.URL, "url", "The URL should match")
							assert.Equal(t, hasCompDetection.ComponentStub.Source.GitSource.DockerfileURL, tt.dockerfileURLMap[hasCompDetection.ComponentStub.Source.GitSource.Context], "The dockerfile URL should match")
						}
					}
				}
			}
		})
	}
}

func TestGetComponentName(t *testing.T) {
	ctx := context.Background()
	fakeClientNoError := NewFakeClient(t)
	fakeClientNoError.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
		return nil
	}
	fakeClientHCExist := NewFakeClient(t)
	fakeClientHCExist.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
		hc := appstudiov1alpha1.Component{
			Spec: appstudiov1alpha1.ComponentSpec{
				ComponentName: "devfile-sample-go-basic",
			},
			Status: appstudiov1alpha1.ComponentStatus{},
		}
		data, _ := json.Marshal(hc)

		json.Unmarshal(data, obj)
		return nil
	}
	fakeClientWithError := NewFakeClient(t)
	fakeClientWithError.MockGet = func(ctx context.Context, key types.NamespacedName, obj client.Object) error {
		return fmt.Errorf("some error")
	}

	tests := []struct {
		name                 string
		client               client.Client
		gitSource            *appstudiov1alpha1.GitSource
		expectedName         string
		expectedRandomString bool
	}{
		{
			name: "valid repo name",
			gitSource: &appstudiov1alpha1.GitSource{
				URL: "https://github.com/devfile-samples/devfile-sample-go-basic",
			},
			client:       fakeClientNoError,
			expectedName: "devfile-sample-go-basic",
		},
		{
			name: "long repo name with special chars",
			gitSource: &appstudiov1alpha1.GitSource{
				URL: "https://github.com/devfile-samples/123-testdevfilego--ImportRepository--withaverylongreporitoryname-test-validation-and-generation",
			},
			client:       fakeClientNoError,
			expectedName: "123-testdevfilego--importrepository--withaverylongreporito",
		},
		{
			name: "numeric repo name",
			gitSource: &appstudiov1alpha1.GitSource{
				URL: "https://github.com/devfile-samples/123454678.git",
			},
			client:       fakeClientNoError,
			expectedName: "comp-123454678",
		},
		{
			name: "error when look for hc",
			gitSource: &appstudiov1alpha1.GitSource{
				URL: "https://github.com/devfile-samples/devfile-sample-go-basic",
			},
			client:               fakeClientWithError,
			expectedName:         "devfile-sample-go-basic",
			expectedRandomString: true,
		},
		{
			name: "hc exist with conflict name",
			gitSource: &appstudiov1alpha1.GitSource{
				URL: "https://github.com/devfile-samples/devfile-sample-go-basic",
			},
			client:               fakeClientHCExist,
			expectedName:         "devfile-sample-go-basic",
			expectedRandomString: true,
		},
		{
			name: "valid repo name with context",
			gitSource: &appstudiov1alpha1.GitSource{
				URL:     "https://github.com/devfile-samples/devfile-multi-component",
				Context: "nodejs",
			},
			client:       fakeClientNoError,
			expectedName: "nodejs-devfile-multi-component",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotComponentName := getComponentName(ctx, tt.gitSource, tt.client, "default")
			if tt.expectedRandomString {
				assert.Contains(t, gotComponentName, tt.expectedName, "the component name should contain repo name")
				assert.NotEqual(t, tt.expectedName, gotComponentName, "the component name should not equal to repo name")
			} else {
				assert.Equal(t, tt.expectedName, gotComponentName, "the component name does not match expected name")
			}
		})
	}

}
