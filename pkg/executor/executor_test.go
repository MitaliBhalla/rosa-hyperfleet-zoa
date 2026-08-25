package executor

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func TestEnsureNamespace_WhenNamespaceDoesNotExist_ItShouldCreateIt(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	err := e.ensureNamespace(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ns, err := client.CoreV1().Namespaces().Get(ctx, jobNamespace, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("namespace not created: %v", err)
	}
	if ns.Labels["app.kubernetes.io/managed-by"] != "zoa-lambda" {
		t.Errorf("expected managed-by label 'zoa-lambda', got %q", ns.Labels["app.kubernetes.io/managed-by"])
	}
}

func TestEnsureNamespace_WhenNamespaceAlreadyExists_ItShouldSucceed(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	err := e.ensureNamespace(ctx)
	if err != nil {
		t.Fatalf("unexpected error when namespace exists: %v", err)
	}
}

func TestEnsureNamespace_WhenCalledTwice_ItShouldBeIdempotent(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	if err := e.ensureNamespace(ctx); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := e.ensureNamespace(ctx); err != nil {
		t.Fatalf("second call should be idempotent: %v", err)
	}
}

func TestCreateResourcesIdempotent_WhenClusterScoped_ItShouldCreateClusterRoleAndBinding(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	rbac := &actions.RBACConfig{
		ClusterScoped: true,
		Rules: []actions.RBACRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list"}},
		},
	}

	err := e.createResourcesIdempotent(ctx, "exec-123", "zoa-exec-exec-123", rbac, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sa, err := client.CoreV1().ServiceAccounts(jobNamespace).Get(ctx, "zoa-exec-exec-123", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("service account not found: %v", err)
	}
	if sa.Labels[labelKey] != "exec-123" {
		t.Fatalf("expected label %s=exec-123, got %s", labelKey, sa.Labels[labelKey])
	}

	cr, err := client.RbacV1().ClusterRoles().Get(ctx, "zoa-exec-exec-123", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("cluster role not found: %v", err)
	}
	if len(cr.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(cr.Rules))
	}

	crb, err := client.RbacV1().ClusterRoleBindings().Get(ctx, "zoa-exec-exec-123", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("cluster role binding not found: %v", err)
	}
	if crb.Subjects[0].Name != "zoa-exec-exec-123" {
		t.Fatalf("expected subject zoa-exec-exec-123, got %s", crb.Subjects[0].Name)
	}
}

func TestCreateResourcesIdempotent_WhenNamespaceScoped_ItShouldCreateRoleAndBinding(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	rbac := &actions.RBACConfig{
		ClusterScoped: false,
		Rules: []actions.RBACRule{
			{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get"}},
		},
	}

	err := e.createResourcesIdempotent(ctx, "exec-456", "zoa-exec-exec-456", rbac, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.RbacV1().Roles(jobNamespace).Get(ctx, "zoa-exec-exec-456", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("role not found: %v", err)
	}

	_, err = client.RbacV1().RoleBindings(jobNamespace).Get(ctx, "zoa-exec-exec-456", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("role binding not found: %v", err)
	}
}

func TestCleanupResources_WhenClusterScoped_ItShouldDeleteAllResources(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "zoa-exec-exec-789", Namespace: jobNamespace}}
	_, _ = client.CoreV1().ServiceAccounts(jobNamespace).Create(ctx, sa, metav1.CreateOptions{})

	cr := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "zoa-exec-exec-789"}}
	_, _ = client.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})

	crb := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "zoa-exec-exec-789"}}
	_, _ = client.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})

	rbac := &actions.RBACConfig{ClusterScoped: true}

	e.cleanupResources(ctx, "exec-789", "zoa-exec-exec-789", rbac, nil)

	_, err := client.CoreV1().ServiceAccounts(jobNamespace).Get(ctx, "zoa-exec-exec-789", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected service account to be deleted")
	}

	_, err = client.RbacV1().ClusterRoles().Get(ctx, "zoa-exec-exec-789", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected cluster role to be deleted")
	}

	_, err = client.RbacV1().ClusterRoleBindings().Get(ctx, "zoa-exec-exec-789", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected cluster role binding to be deleted")
	}
}

func TestCleanupResources_WhenResourcesDoNotExist_ItShouldNotError(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	rbac := &actions.RBACConfig{ClusterScoped: true}
	e.cleanupResources(ctx, "nonexistent", "zoa-exec-nonexistent", rbac, nil)
}

func TestCreateResourcesIdempotent_WhenNilRBAC_ItShouldOnlyCreateServiceAccount(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	err := e.createResourcesIdempotent(ctx, "exec-nil", "zoa-exec-exec-nil", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = client.CoreV1().ServiceAccounts(jobNamespace).Get(ctx, "zoa-exec-exec-nil", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("service account not found: %v", err)
	}
}

func TestCreateResourcesIdempotent_WhenCalledTwice_ItShouldBeIdempotent(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs
	e := &Executor{kubeClient: client, logger: noopLogger()}
	ctx := context.Background()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: jobNamespace}}
	_, _ = client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})

	rbac := &actions.RBACConfig{
		ClusterScoped: true,
		Rules: []actions.RBACRule{
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}},
		},
	}

	if err := e.createResourcesIdempotent(ctx, "exec-idem", "zoa-exec-exec-idem", rbac, nil); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := e.createResourcesIdempotent(ctx, "exec-idem", "zoa-exec-exec-idem", rbac, nil); err != nil {
		t.Fatalf("second call should be idempotent: %v", err)
	}
}
