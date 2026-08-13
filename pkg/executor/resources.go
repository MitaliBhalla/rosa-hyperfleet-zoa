package executor

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/openshift-online/rosa-hyperfleet-zoa/pkg/actions"
)

func (e *Executor) ensureNamespace(ctx context.Context) error {
	_, err := e.kubeClient.CoreV1().Namespaces().Get(ctx, jobNamespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("checking namespace %q: %w", jobNamespace, err)
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "zoa-lambda",
			},
		},
	}
	_, err = e.kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating namespace %q: %w", jobNamespace, err)
	}
	e.logger.Info("created namespace", "namespace", jobNamespace)
	return nil
}

func (e *Executor) createResourcesIdempotent(ctx context.Context, executionID, saName string, rbac *actions.RBACConfig, params map[string]string) error {
	if rbac != nil {
		if err := actions.ValidateRBACRules(rbac); err != nil {
			return fmt.Errorf("RBAC validation failed: %w", err)
		}
	}
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      saName,
			Namespace: jobNamespace,
			Labels: map[string]string{
				labelKey: executionID,
			},
		},
	}
	if err := e.withRetry(ctx, "create-service-account", func() error {
		_, err := e.kubeClient.CoreV1().ServiceAccounts(jobNamespace).Create(ctx, sa, metav1.CreateOptions{})
		return err
	}); err != nil {
		return fmt.Errorf("creating service account: %w", err)
	}

	if rbac == nil {
		return nil
	}

	rules := make([]rbacv1.PolicyRule, 0, len(rbac.Rules))
	for _, r := range rbac.Rules {
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups: r.APIGroups,
			Resources: r.Resources,
			Verbs:     r.Verbs,
		})
	}

	if rbac.ClusterScoped {
		roleName := fmt.Sprintf("zoa-exec-%s", executionID)
		cr := &rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{
				Name: roleName,
				Labels: map[string]string{
					labelKey: executionID,
				},
			},
			Rules: rules,
		}
		if err := e.withRetry(ctx, "create-cluster-role", func() error {
			_, err := e.kubeClient.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{})
			return err
		}); err != nil {
			return fmt.Errorf("creating cluster role: %w", err)
		}

		crb := &rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: roleName,
				Labels: map[string]string{
					labelKey: executionID,
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: jobNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "ClusterRole",
				Name:     roleName,
			},
		}
		if err := e.withRetry(ctx, "create-cluster-role-binding", func() error {
			_, err := e.kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{})
			return err
		}); err != nil {
			return fmt.Errorf("creating cluster role binding: %w", err)
		}
	} else {
		targetNS := jobNamespace
		if rbac.NamespaceParam != "" {
			if v, ok := params[rbac.NamespaceParam]; ok && v != "" {
				targetNS = v
			}
		}

		roleName := fmt.Sprintf("zoa-exec-%s", executionID)
		role := &rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: targetNS,
				Labels: map[string]string{
					labelKey: executionID,
				},
			},
			Rules: rules,
		}
		if err := e.withRetry(ctx, "create-role", func() error {
			_, err := e.kubeClient.RbacV1().Roles(targetNS).Create(ctx, role, metav1.CreateOptions{})
			return err
		}); err != nil {
			return fmt.Errorf("creating role: %w", err)
		}

		rb := &rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      roleName,
				Namespace: targetNS,
				Labels: map[string]string{
					labelKey: executionID,
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: jobNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "Role",
				Name:     roleName,
			},
		}
		if err := e.withRetry(ctx, "create-role-binding", func() error {
			_, err := e.kubeClient.RbacV1().RoleBindings(targetNS).Create(ctx, rb, metav1.CreateOptions{})
			return err
		}); err != nil {
			return fmt.Errorf("creating role binding: %w", err)
		}
	}

	return nil
}

func (e *Executor) cleanupResources(ctx context.Context, executionID, saName string, rbac *actions.RBACConfig, params map[string]string) {
	logger := e.logger.With("execution_id", executionID)

	if err := e.kubeClient.CoreV1().ServiceAccounts(jobNamespace).Delete(ctx, saName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		logger.Warn("failed to delete service account", "error", err)
	}

	// Delete STS credentials Secret (created for async Jobs)
	credsSecretName := fmt.Sprintf("zoa-creds-%s", executionID)
	if err := e.kubeClient.CoreV1().Secrets(jobNamespace).Delete(ctx, credsSecretName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		logger.Warn("failed to delete credentials secret", "error", err)
	}

	if rbac == nil {
		return
	}

	roleName := fmt.Sprintf("zoa-exec-%s", executionID)

	if rbac.ClusterScoped {
		if err := e.kubeClient.RbacV1().ClusterRoleBindings().Delete(ctx, roleName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			logger.Warn("failed to delete cluster role binding", "error", err)
		}
		if err := e.kubeClient.RbacV1().ClusterRoles().Delete(ctx, roleName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			logger.Warn("failed to delete cluster role", "error", err)
		}
	} else {
		targetNS := jobNamespace
		if rbac.NamespaceParam != "" {
			if v, ok := params[rbac.NamespaceParam]; ok && v != "" {
				targetNS = v
			}
		}
		if err := e.kubeClient.RbacV1().RoleBindings(targetNS).Delete(ctx, roleName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			logger.Warn("failed to delete role binding", "error", err)
		}
		if err := e.kubeClient.RbacV1().Roles(targetNS).Delete(ctx, roleName, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			logger.Warn("failed to delete role", "error", err)
		}
	}
}

func (e *Executor) clientForServiceAccount(_ context.Context, saName string) (kubernetes.Interface, dynamic.Interface, *rest.Config, error) {
	impersonateConfig := rest.CopyConfig(e.restConfig)
	impersonateConfig.Impersonate = rest.ImpersonationConfig{
		UserName: fmt.Sprintf("system:serviceaccount:%s:%s", jobNamespace, saName),
	}
	impersonateConfig.Timeout = 30 * time.Second

	client, err := kubernetes.NewForConfig(impersonateConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating impersonated client: %w", err)
	}

	dynClient, err := dynamic.NewForConfig(impersonateConfig)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("creating impersonated dynamic client: %w", err)
	}

	return client, dynClient, impersonateConfig, nil
}
