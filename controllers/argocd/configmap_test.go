// Copyright 2020 ArgoCD Operator Developers
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package argocd

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	argoprojv1alpha1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	"github.com/argoproj-labs/argocd-operator/common"
	"github.com/argoproj-labs/argocd-operator/controllers/argoutil"
)

var _ reconcile.Reconciler = &ReconcileArgoCD{}

func TestReconcileArgoCD_reconcileTLSCerts(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD(initialCerts(t, "root-ca.example.com"))
	r := makeTestReconciler(t, a)

	assert.NoError(t, r.reconcileTLSCerts(a))

	configMap := &corev1.ConfigMap{}
	assert.NoError(t, r.Client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      common.ArgoCDTLSCertsConfigMapName,
			Namespace: a.Namespace,
		},
		configMap))

	want := []string{"root-ca.example.com"}
	if k := stringMapKeys(configMap.Data); !reflect.DeepEqual(want, k) {
		t.Fatalf("got %#v, want %#v\n", k, want)
	}
}

func TestReconcileArgoCD_reconcileTLSCerts_configMapUpdate(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD(initialCerts(t, "root-ca.example.com"))
	r := makeTestReconciler(t, a)

	assert.NoError(t, r.reconcileTLSCerts(a))

	configMap := &corev1.ConfigMap{}
	assert.NoError(t, r.Client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      common.ArgoCDTLSCertsConfigMapName,
			Namespace: a.Namespace,
		},
		configMap))

	want := []string{"root-ca.example.com"}
	if k := stringMapKeys(configMap.Data); !reflect.DeepEqual(want, k) {
		t.Fatalf("got %#v, want %#v\n", k, want)
	}

	// update a new cert in argocd-tls-certs-cm
	testPEM := generateEncodedPEM(t, "example.com")

	configMap.Data["example.com"] = string(testPEM)
	assert.NoError(t, r.Client.Update(context.TODO(), configMap))

	// verify that a new reconciliation does not remove example.com from
	// argocd-tls-certs-cm
	assert.NoError(t, r.reconcileTLSCerts(a))

	want = []string{"example.com", "root-ca.example.com"}
	if k := stringMapKeys(configMap.Data); !reflect.DeepEqual(want, k) {
		t.Fatalf("got %#v, want %#v\n", k, want)
	}
}

func TestReconcileArgoCD_reconcileTLSCerts_withInitialCertsUpdate(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)
	assert.NoError(t, r.reconcileTLSCerts(a))

	a = makeTestArgoCD(initialCerts(t, "testing.example.com"))
	assert.NoError(t, r.reconcileTLSCerts(a))

	configMap := &corev1.ConfigMap{}
	assert.NoError(t, r.Client.Get(
		context.TODO(),
		types.NamespacedName{
			Name:      common.ArgoCDTLSCertsConfigMapName,
			Namespace: a.Namespace,
		},
		configMap))

	// Any certs added to .spec.tls.intialCerts of Argo CD CR after the cluster creation
	// should not affect the argocd-tls-certs-cm configmap.
	want := []string{}
	if k := stringMapKeys(configMap.Data); !reflect.DeepEqual(want, k) {
		t.Fatalf("got %#v, want %#v\n", k, want)
	}
}

func TestReconcileArgoCD_reconcileArgoConfigMap(t *testing.T) {
	logf.SetLogger(ZapLogger(true))

	defaultConfigMapData := map[string]string{
		"application.instanceLabelKey":       common.ArgoCDDefaultApplicationInstanceLabelKey,
		"application.resourceTrackingMethod": argoprojv1alpha1.ResourceTrackingMethodLabel.String(),
		"admin.enabled":                      "true",
		"configManagementPlugins":            "",
		"dex.config":                         "",
		"ga.anonymizeusers":                  "false",
		"ga.trackingid":                      "",
		"help.chatText":                      "Chat now!",
		"help.chatUrl":                       "https://mycorp.slack.com/argo-cd",
		"kustomize.buildOptions":             "",
		"oidc.config":                        "",
		"repositories":                       "",
		"repository.credentials":             "",
		"resource.inclusions":                "",
		"resource.exclusions":                "",
		"statusbadge.enabled":                "false",
		"url":                                "https://argocd-server",
		"users.anonymous.enabled":            "false",
	}

	cmdTests := []struct {
		name     string
		opts     []argoCDOpt
		dataDiff map[string]string
	}{
		{
			"defaults",
			[]argoCDOpt{},
			map[string]string{},
		},
		{
			"with-banner",
			[]argoCDOpt{func(a *argoprojv1alpha1.ArgoCD) {
				a.Spec.Banner = &argoprojv1alpha1.Banner{
					Content: "Custom Styles - Banners",
				}
			}},
			map[string]string{
				"users.anonymous.enabled": "false",
				"ui.bannercontent":        "Custom Styles - Banners",
			},
		},
		{
			"with-banner-and-url",
			[]argoCDOpt{func(a *argoprojv1alpha1.ArgoCD) {
				a.Spec.Banner = &argoprojv1alpha1.Banner{
					Content: "Custom Styles - Banners",
					URL:     "https://argo-cd.readthedocs.io/en/stable/operator-manual/custom-styles/#banners",
				}
			}},
			map[string]string{
				"ui.bannercontent": "Custom Styles - Banners",
				"ui.bannerurl":     "https://argo-cd.readthedocs.io/en/stable/operator-manual/custom-styles/#banners",
			},
		},
	}

	for _, tt := range cmdTests {
		a := makeTestArgoCD(tt.opts...)
		r := makeTestReconciler(t, a)

		err := r.reconcileArgoConfigMap(a)
		assert.NoError(t, err)

		cm := &corev1.ConfigMap{}
		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      common.ArgoCDConfigMapName,
			Namespace: testNamespace,
		}, cm)
		assert.NoError(t, err)

		want := merge(defaultConfigMapData, tt.dataDiff)

		if diff := cmp.Diff(want, cm.Data); diff != "" {
			t.Fatalf("reconcileArgoConfigMap (%s) failed:\n%s", tt.name, diff)
		}
	}
}

func TestReconcileArgoCD_reconcileEmptyArgoConfigMap(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)

	// An empty Argo CD Configmap
	emptyArgoConfigmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.ArgoCDConfigMapName,
			Namespace: a.Namespace,
		},
	}

	err := r.Client.Create(context.TODO(), emptyArgoConfigmap)
	assert.NoError(t, err)

	err = r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)
}

func TestReconcileArgoCDCM_withRepoCredentials(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD()
	a.Spec.RepositoryCredentials = `
- url: https://github.com/test/gitops.git
  passwordSecret:
    name: test
    key: password
  usernameSecret:
    name: test
    key: username`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.ArgoCDConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			"application.instanceLabelKey": "mycompany.com/appname",
			"admin.enabled":                "true",
		},
	}
	r := makeTestReconciler(t, a, cm)

	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	if got := cm.Data[common.ArgoCDKeyRepositoryCredentials]; got != a.Spec.RepositoryCredentials {
		t.Fatalf("reconcileArgoConfigMap failed: got %s, want %s", got, a.Spec.RepositoryCredentials)
	}
}

func TestReconcileArgoCD_reconcileArgoConfigMap_withDisableAdmin(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD(func(a *argoprojv1alpha1.ArgoCD) {
		a.Spec.DisableAdmin = true
	})
	r := makeTestReconciler(t, a)

	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	if c := cm.Data["admin.enabled"]; c != "false" {
		t.Fatalf("reconcileArgoConfigMap failed got %q, want %q", c, "false")
	}
}

func TestReconcileArgoCD_reconcileArgoConfigMap_withDexConnector(t *testing.T) {
	restoreEnv(t)
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD(func(a *argoprojv1alpha1.ArgoCD) {
		a.Spec.Dex.OpenShiftOAuth = true
	})
	sa := &corev1.ServiceAccount{
		TypeMeta:   metav1.TypeMeta{Kind: "ServiceAccount", APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{Name: "argocd-argocd-dex-server", Namespace: "argocd"},
		Secrets: []corev1.ObjectReference{{
			Name: "token",
		}},
	}

	secret := argoutil.NewSecretWithName(a, "token")
	r := makeTestReconciler(t, a, sa, secret)
	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	dex, ok := cm.Data["dex.config"]
	if !ok {
		t.Fatal("reconcileArgoConfigMap with dex failed")
	}

	m := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(dex), &m)
	assert.NoError(t, err, fmt.Sprintf("failed to unmarshal %s", dex))

	connectors, ok := m["connectors"]
	if !ok {
		t.Fatal("no connectors found in dex.config")
	}
	dexConnector := connectors.([]interface{})[0].(map[interface{}]interface{})
	config := dexConnector["config"]
	assert.Equal(t, config.(map[interface{}]interface{})["clientID"], "system:serviceaccount:argocd:argocd-argocd-dex-server")
}

func TestReconcileArgoCD_reconcileArgoConfigMap_withDexDisabled(t *testing.T) {
	restoreEnv(t)
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)

	os.Setenv("DISABLE_DEX", "true")
	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	if c, ok := cm.Data["dex.config"]; ok {
		t.Fatalf("reconcileArgoConfigMap failed, dex.config = %q", c)
	}
}
func TestReconcileArgoCD_reconcileArgoConfigMap_withMultipleSSOConfigured(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCDForKeycloakWithDex()
	r := makeTestReconciler(t, a)

	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	if c, ok := cm.Data["dex.openShiftOAuth"]; !ok && len(c) != 0 {
		t.Fatalf("reconcileArgoConfigMap didn't skip setting dex when keycloak is configured, dex.openShiftOAuth = %q", c)
	}
}

func TestReconcileArgoCD_reconcileArgoConfigMap_withKustomizeVersions(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD(func(a *argoprojv1alpha1.ArgoCD) {
		kv := argoprojv1alpha1.KustomizeVersionSpec{
			Version: "v4.1.0",
			Path:    "/path/to/kustomize-4.1",
		}
		var kvs []argoprojv1alpha1.KustomizeVersionSpec
		kvs = append(kvs, kv)
		a.Spec.KustomizeVersions = kvs
	})
	r := makeTestReconciler(t, a)

	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	if diff := cmp.Diff(cm.Data["kustomize.version.v4.1.0"], "/path/to/kustomize-4.1"); diff != "" {
		t.Fatalf("failed to reconcile configmap:\n%s", diff)
	}
}

func TestReconcileArgoCD_reconcileGPGKeysConfigMap(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD(func(a *argoprojv1alpha1.ArgoCD) {
		a.Spec.DisableAdmin = true
	})
	r := makeTestReconciler(t, a)

	err := r.reconcileGPGKeysConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDGPGKeysConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)
	// Currently the gpg keys configmap is empty
}

func TestReconcileArgoCD_reconcileArgoConfigMap_withResourceTrackingMethod(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	a := makeTestArgoCD()
	r := makeTestReconciler(t, a)

	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}

	t.Run("Check default tracking method", func(t *testing.T) {
		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      common.ArgoCDConfigMapName,
			Namespace: testNamespace,
		}, cm)
		assert.NoError(t, err)

		rtm, ok := cm.Data[common.ArgoCDKeyResourceTrackingMethod]
		assert.Equal(t, argoprojv1alpha1.ResourceTrackingMethodLabel.String(), rtm)
		assert.True(t, ok)
	})

	t.Run("Tracking method label", func(t *testing.T) {
		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      common.ArgoCDConfigMapName,
			Namespace: testNamespace,
		}, cm)
		assert.NoError(t, err)

		rtm, ok := cm.Data[common.ArgoCDKeyResourceTrackingMethod]
		assert.Equal(t, argoprojv1alpha1.ResourceTrackingMethodLabel.String(), rtm)
		assert.True(t, ok)
	})

	t.Run("Set tracking method to annotation+label", func(t *testing.T) {
		a.Spec.ResourceTrackingMethod = argoprojv1alpha1.ResourceTrackingMethodAnnotationAndLabel.String()
		err = r.reconcileArgoConfigMap(a)
		assert.NoError(t, err)

		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      common.ArgoCDConfigMapName,
			Namespace: testNamespace,
		}, cm)
		assert.NoError(t, err)

		rtm, ok := cm.Data[common.ArgoCDKeyResourceTrackingMethod]
		assert.True(t, ok)
		assert.Equal(t, argoprojv1alpha1.ResourceTrackingMethodAnnotationAndLabel.String(), rtm)
	})

	t.Run("Set tracking method to annotation", func(t *testing.T) {
		a.Spec.ResourceTrackingMethod = argoprojv1alpha1.ResourceTrackingMethodAnnotation.String()
		err = r.reconcileArgoConfigMap(a)
		assert.NoError(t, err)

		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      common.ArgoCDConfigMapName,
			Namespace: testNamespace,
		}, cm)
		assert.NoError(t, err)

		rtm, ok := cm.Data[common.ArgoCDKeyResourceTrackingMethod]
		assert.True(t, ok)
		assert.Equal(t, argoprojv1alpha1.ResourceTrackingMethodAnnotation.String(), rtm)
	})

	// Invalid value sets the default "label"
	t.Run("Set tracking method to invalid value", func(t *testing.T) {
		a.Spec.ResourceTrackingMethod = "anotaions"
		err = r.reconcileArgoConfigMap(a)
		assert.NoError(t, err)

		err = r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      common.ArgoCDConfigMapName,
			Namespace: testNamespace,
		}, cm)
		assert.NoError(t, err)

		rtm, ok := cm.Data[common.ArgoCDKeyResourceTrackingMethod]
		assert.True(t, ok)
		assert.Equal(t, argoprojv1alpha1.ResourceTrackingMethodLabel.String(), rtm)
	})

}

func TestReconcileArgoCD_reconcileArgoConfigMap_withResourceInclusions(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	customizations := "testing: testing"
	updatedCustomizations := "updated-testing: updated-testing"

	a := makeTestArgoCD(func(a *argoprojv1alpha1.ArgoCD) {
		a.Spec.ResourceInclusions = customizations
	})
	r := makeTestReconciler(t, a)

	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	if c := cm.Data["resource.inclusions"]; c != customizations {
		t.Fatalf("reconcileArgoConfigMap failed got %q, want %q", c, customizations)
	}

	a.Spec.ResourceInclusions = updatedCustomizations
	err = r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	if c := cm.Data["resource.inclusions"]; c != updatedCustomizations {
		t.Fatalf("reconcileArgoConfigMap failed got %q, want %q", c, updatedCustomizations)
	}

}

func TestReconcileArgoCD_reconcileArgoConfigMap_withResourceCustomizations(t *testing.T) {
	logf.SetLogger(ZapLogger(true))
	customizations := "testing: testing"
	a := makeTestArgoCD(func(a *argoprojv1alpha1.ArgoCD) {
		a.Spec.ResourceCustomizations = customizations
	})
	r := makeTestReconciler(t, a)

	err := r.reconcileArgoConfigMap(a)
	assert.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      common.ArgoCDConfigMapName,
		Namespace: testNamespace,
	}, cm)
	assert.NoError(t, err)

	if c := cm.Data["resource.customizations"]; c != customizations {
		t.Fatalf("reconcileArgoConfigMap failed got %q, want %q", c, customizations)
	}
}
