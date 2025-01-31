package argocd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/google/go-cmp/cmp"
	routev1 "github.com/openshift/api/route/v1"

	argov1alpha1 "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
)

func TestReconcileRouteSetLabels(t *testing.T) {
	routeAPIFound = true
	ctx := context.Background()
	logf.SetLogger(ZapLogger(true))
	argoCD := makeArgoCD(func(a *argov1alpha1.ArgoCD) {
		a.Spec.Server.Route.Enabled = true
		labels := make(map[string]string)
		labels["my-key"] = "my-value"
		a.Spec.Server.Route.Labels = labels
	})
	objs := []runtime.Object{
		argoCD,
	}
	r := makeReconciler(t, argoCD, objs...)
	assert.NoError(t, createNamespace(r, argoCD.Namespace, ""))

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      testArgoCDName,
			Namespace: testNamespace,
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	assert.NoError(t, err)

	loaded := &routev1.Route{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: testArgoCDName + "-server", Namespace: testNamespace}, loaded)
	fatalIfError(t, err, "failed to load route %q: %s", testArgoCDName+"-server", err)

	if diff := cmp.Diff("my-value", loaded.Labels["my-key"]); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}

}
func TestReconcileRouteSetsInsecure(t *testing.T) {
	routeAPIFound = true
	ctx := context.Background()
	logf.SetLogger(ZapLogger(true))
	argoCD := makeArgoCD(func(a *argov1alpha1.ArgoCD) {
		a.Spec.Server.Route.Enabled = true
	})
	objs := []runtime.Object{
		argoCD,
	}
	r := makeReconciler(t, argoCD, objs...)
	assert.NoError(t, createNamespace(r, argoCD.Namespace, ""))

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      testArgoCDName,
			Namespace: testNamespace,
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	assert.NoError(t, err)

	loaded := &routev1.Route{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: testArgoCDName + "-server", Namespace: testNamespace}, loaded)
	fatalIfError(t, err, "failed to load route %q: %s", testArgoCDName+"-server", err)

	wantTLSConfig := &routev1.TLSConfig{
		Termination:                   routev1.TLSTerminationPassthrough,
		InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
	}
	if diff := cmp.Diff(wantTLSConfig, loaded.Spec.TLS); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}
	wantPort := &routev1.RoutePort{
		TargetPort: intstr.FromString("https"),
	}
	if diff := cmp.Diff(wantPort, loaded.Spec.Port); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}

	// second reconciliation after changing the Insecure flag.
	err = r.Client.Get(ctx, req.NamespacedName, argoCD)
	fatalIfError(t, err, "failed to load ArgoCD %q: %s", testArgoCDName+"-server", err)

	argoCD.Spec.Server.Insecure = true
	err = r.Client.Update(ctx, argoCD)
	fatalIfError(t, err, "failed to update the ArgoCD: %s", err)

	_, err = r.Reconcile(context.TODO(), req)
	fatalIfError(t, err, "reconcile: (%v): %s", req, err)

	loaded = &routev1.Route{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: testArgoCDName + "-server", Namespace: testNamespace}, loaded)
	fatalIfError(t, err, "failed to load route %q: %s", testArgoCDName+"-server", err)

	wantTLSConfig = &routev1.TLSConfig{
		InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		Termination:                   routev1.TLSTerminationEdge,
	}
	if diff := cmp.Diff(wantTLSConfig, loaded.Spec.TLS); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}
	wantPort = &routev1.RoutePort{
		TargetPort: intstr.FromString("http"),
	}
	if diff := cmp.Diff(wantPort, loaded.Spec.Port); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}
}

func TestReconcileRouteUnsetsInsecure(t *testing.T) {
	routeAPIFound = true
	ctx := context.Background()
	logf.SetLogger(ZapLogger(true))
	argoCD := makeArgoCD(func(a *argov1alpha1.ArgoCD) {
		a.Spec.Server.Route.Enabled = true
		a.Spec.Server.Insecure = true
	})
	objs := []runtime.Object{
		argoCD,
	}
	r := makeReconciler(t, argoCD, objs...)
	assert.NoError(t, createNamespace(r, argoCD.Namespace, ""))

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      testArgoCDName,
			Namespace: testNamespace,
		},
	}

	_, err := r.Reconcile(context.TODO(), req)
	assert.NoError(t, err)

	loaded := &routev1.Route{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: testArgoCDName + "-server", Namespace: testNamespace}, loaded)
	fatalIfError(t, err, "failed to load route %q: %s", testArgoCDName+"-server", err)

	wantTLSConfig := &routev1.TLSConfig{
		InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
		Termination:                   routev1.TLSTerminationEdge,
	}
	if diff := cmp.Diff(wantTLSConfig, loaded.Spec.TLS); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}
	wantPort := &routev1.RoutePort{
		TargetPort: intstr.FromString("http"),
	}
	if diff := cmp.Diff(wantPort, loaded.Spec.Port); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}

	// second reconciliation after changing the Insecure flag.
	err = r.Client.Get(ctx, req.NamespacedName, argoCD)
	fatalIfError(t, err, "failed to load ArgoCD %q: %s", testArgoCDName+"-server", err)

	argoCD.Spec.Server.Insecure = false
	err = r.Client.Update(ctx, argoCD)
	fatalIfError(t, err, "failed to update the ArgoCD: %s", err)

	_, err = r.Reconcile(context.TODO(), req)
	assert.NoError(t, err)

	loaded = &routev1.Route{}
	err = r.Client.Get(ctx, types.NamespacedName{Name: testArgoCDName + "-server", Namespace: testNamespace}, loaded)
	fatalIfError(t, err, "failed to load route %q: %s", testArgoCDName+"-server", err)

	wantTLSConfig = &routev1.TLSConfig{
		Termination:                   routev1.TLSTerminationPassthrough,
		InsecureEdgeTerminationPolicy: routev1.InsecureEdgeTerminationPolicyRedirect,
	}
	if diff := cmp.Diff(wantTLSConfig, loaded.Spec.TLS); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}
	wantPort = &routev1.RoutePort{
		TargetPort: intstr.FromString("https"),
	}
	if diff := cmp.Diff(wantPort, loaded.Spec.Port); diff != "" {
		t.Fatalf("failed to reconcile route:\n%s", diff)
	}
}

func makeReconciler(t *testing.T, acd *argov1alpha1.ArgoCD, objs ...runtime.Object) *ReconcileArgoCD {
	t.Helper()
	s := scheme.Scheme
	s.AddKnownTypes(argov1alpha1.GroupVersion, acd)
	routev1.Install(s)
	cl := fake.NewFakeClient(objs...)
	return &ReconcileArgoCD{
		Client: cl,
		Scheme: s,
	}
}

func makeArgoCD(opts ...func(*argov1alpha1.ArgoCD)) *argov1alpha1.ArgoCD {
	argoCD := &argov1alpha1.ArgoCD{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testArgoCDName,
			Namespace: testNamespace,
		},
		Spec: argov1alpha1.ArgoCDSpec{},
	}
	for _, o := range opts {
		o(argoCD)
	}
	return argoCD
}

func fatalIfError(t *testing.T, err error, format string, a ...interface{}) {
	t.Helper()
	if err != nil {
		t.Fatalf(format, a...)
	}
}

func loadSecret(t *testing.T, c client.Client, name string) *corev1.Secret {
	t.Helper()
	secret := &corev1.Secret{}
	err := c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, secret)
	fatalIfError(t, err, "failed to load secret %q", name)
	return secret
}

func testNamespacedName(name string) types.NamespacedName {
	return types.NamespacedName{
		Name:      name,
		Namespace: testNamespace,
	}
}
