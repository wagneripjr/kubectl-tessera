//go:build e2e

package drivers

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/utils/ptr"
)

// External observable contract (ADR-008). Defined here, NOT imported from
// internal/labels, so the acceptance suite stays black-box (ATDD Gate G4): a change
// to the SUT's label keys SHOULD break these tests.
const (
	managedByKey   = "app.kubernetes.io/managed-by"
	managedByValue = "kubectl-tessera"
	sessionIDKey   = "tessera.adustio.com/session-id"
	ownerKey       = "tessera.adustio.com/owner"
	expiresAtKey   = "tessera.adustio.com/expires-at"
)

var sessionIDRe = regexp.MustCompile(`session-id[=:\s]+([a-z0-9-]+)`)

// KindDriver satisfies the documented protocol-driver contract.
var _ TesseraDriver = (*KindDriver)(nil)

// KindDriver is the composite protocol driver: process adapter (the binary) +
// cluster adapter (client-go against a real kind API server).
type KindDriver struct {
	binaryPath string
	workDir    string
	namespace  string // per-scenario namespace, set by the suite Before hook

	adminConfig  *rest.Config
	admin        *kubernetes.Clientset
	adminDynamic dynamic.Interface
	mapper       meta.RESTMapper

	identities map[string]string // limited-identity name -> kubeconfig path
	lastExec   *exec.Cmd
}

// NewKindDriver builds the binary once and connects to the kind API server. A
// failure here is a broken harness, not a valid RED.
func NewKindDriver(ctx context.Context) (*KindDriver, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	workDir, err := os.MkdirTemp("", "tessera-e2e-")
	if err != nil {
		return nil, err
	}
	binaryPath := filepath.Join(workDir, "kubectl-tessera")
	build := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, "./cmd/kubectl-tessera")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("building binary: %v: %s", err, out)
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kube config (is kind reachable?): %w", err)
	}
	admin, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	groupResources, err := restmapper.GetAPIGroupResources(admin.Discovery())
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}

	return &KindDriver{
		binaryPath:   binaryPath,
		workDir:      workDir,
		adminConfig:  cfg,
		admin:        admin,
		adminDynamic: dyn,
		mapper:       restmapper.NewDiscoveryRESTMapper(groupResources),
		identities:   map[string]string{},
	}, nil
}

// SetNamespace sets the active per-scenario namespace.
func (d *KindDriver) SetNamespace(ns string) { d.namespace = ns }

// Namespace returns the active per-scenario namespace.
func (d *KindDriver) Namespace() string { return d.namespace }

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// --- process adapter ---

func (d *KindDriver) Mint(ctx context.Context, req MintRequest) (MintResult, error) {
	args := []string{"--resource", strings.Join(req.Resources, ",")}
	if len(req.Verbs) > 0 {
		args = append(args, "--verb", strings.Join(req.Verbs, ","))
	}
	if len(req.Namespaces) > 0 {
		args = append(args, "--namespace", req.Namespaces[0])
	} else if d.namespace != "" && !req.ClusterScoped {
		args = append(args, "--namespace", d.namespace)
	}
	if len(req.ResourceNames) > 0 {
		args = append(args, "--resource-name", strings.Join(req.ResourceNames, ","))
	}
	if req.APIGroup != "" {
		args = append(args, "--api-group", req.APIGroup)
	}
	if req.TTL > 0 {
		args = append(args, "--ttl", req.TTL.String())
	}
	if req.ClusterScoped {
		args = append(args, "--cluster-scoped")
	}
	switch req.Mode {
	case ModePrintKubeconfig:
		args = append(args, "--print-kubeconfig")
	case ModeDryRun:
		args = append(args, "--dry-run")
	case ModeExec:
		args = append(args, "--exec")
	}
	if req.AsIdentity != "" {
		args = append(args, "--kubeconfig", d.identities[req.AsIdentity])
	}
	return d.runBinary(ctx, req.Mode == ModeExec, args...)
}

func (d *KindDriver) Gc(ctx context.Context) (MintResult, error) {
	return d.runBinary(ctx, false, "gc")
}

func (d *KindDriver) Ls(ctx context.Context) (MintResult, error) {
	return d.runBinary(ctx, false, "ls")
}

// runBinary spawns the SUT and captures its observable output. A non-zero exit is
// DATA in the result, not a Go error — only a spawn failure returns an error.
func (d *KindDriver) runBinary(ctx context.Context, isExec bool, args ...string) (MintResult, error) {
	cmd := exec.CommandContext(ctx, d.binaryPath, args...)
	// Inherit KUBECONFIG so admin-context runs use the kind cluster.
	cmd.Env = os.Environ()
	if isExec {
		// Testability: drive --exec without a TTY by pointing SHELL at a no-op that
		// exits immediately, and closing stdin (docs/design/protocol-drivers.md).
		cmd.Env = append(cmd.Env, "SHELL=/usr/bin/true")
		cmd.Stdin = nil
	}
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if isExec {
		d.lastExec = cmd
	}

	res := MintResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			return res, fmt.Errorf("spawning binary: %w", err)
		}
	}
	if m := sessionIDRe.FindStringSubmatch(res.Stderr); m != nil {
		res.SessionID = m[1]
	}
	if strings.Contains(strings.Join(args, " "), "--print-kubeconfig") && res.ExitCode == 0 {
		res.KubeconfigPath = strings.TrimSpace(res.Stdout)
	}
	return res, nil
}

func (d *KindDriver) KillExecProcess(_ context.Context, signal string) error {
	if d.lastExec == nil || d.lastExec.Process == nil {
		return fmt.Errorf("no exec process to kill")
	}
	sig := syscall.SIGKILL
	if signal != "SIGKILL" && signal != "-9" {
		sig = syscall.SIGTERM
	}
	return d.lastExec.Process.Signal(sig)
}

// --- cluster adapter ---

func (d *KindDriver) SessionObjectsCount(ctx context.Context, sessionID string) (int, error) {
	sel := fmt.Sprintf("%s=%s,%s=%s", managedByKey, managedByValue, sessionIDKey, sessionID)
	opts := metav1.ListOptions{LabelSelector: sel}
	count := 0
	sas, err := d.admin.CoreV1().ServiceAccounts(metav1.NamespaceAll).List(ctx, opts)
	if err != nil {
		return 0, err
	}
	count += len(sas.Items)
	roles, err := d.admin.RbacV1().Roles(metav1.NamespaceAll).List(ctx, opts)
	if err != nil {
		return 0, err
	}
	count += len(roles.Items)
	rbs, err := d.admin.RbacV1().RoleBindings(metav1.NamespaceAll).List(ctx, opts)
	if err != nil {
		return 0, err
	}
	count += len(rbs.Items)
	crs, err := d.admin.RbacV1().ClusterRoles().List(ctx, opts)
	if err != nil {
		return 0, err
	}
	count += len(crs.Items)
	crbs, err := d.admin.RbacV1().ClusterRoleBindings().List(ctx, opts)
	if err != nil {
		return 0, err
	}
	count += len(crbs.Items)
	return count, nil
}

func (d *KindDriver) SessionObjectsExist(ctx context.Context, sessionID string) (bool, error) {
	n, err := d.SessionObjectsCount(ctx, sessionID)
	return n > 0, err
}

// CountManaged counts tessera-managed objects in the active namespace. Used to
// assert "no managed objects were created" when no session-id is known (e.g. a
// pre-flight refusal). Not part of TesseraDriver — a concrete harness helper.
func (d *KindDriver) CountManaged(ctx context.Context) (int, error) {
	opts := metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", managedByKey, managedByValue)}
	count := 0
	sas, err := d.admin.CoreV1().ServiceAccounts(d.namespace).List(ctx, opts)
	if err != nil {
		return 0, err
	}
	count += len(sas.Items)
	roles, err := d.admin.RbacV1().Roles(d.namespace).List(ctx, opts)
	if err != nil {
		return 0, err
	}
	count += len(roles.Items)
	rbs, err := d.admin.RbacV1().RoleBindings(d.namespace).List(ctx, opts)
	if err != nil {
		return 0, err
	}
	count += len(rbs.Items)
	return count, nil
}

func (d *KindDriver) UnmanagedRBACExists(ctx context.Context, name string) (bool, error) {
	_, err := d.admin.RbacV1().RoleBindings(d.namespace).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	return err == nil, err
}

func (d *KindDriver) MintedTokenCan(ctx context.Context, kubeconfigPath, verb, resource, group, namespace, name string) (bool, error) {
	cs, err := clientsetFromKubeconfig(kubeconfigPath)
	if err != nil {
		return false, err
	}
	ssar := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb: verb, Group: group, Resource: resource, Namespace: namespace, Name: name,
			},
		},
	}
	out, err := cs.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, ssar, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return out.Status.Allowed, nil
}

func (d *KindDriver) MintedTokenRequest(ctx context.Context, kubeconfigPath, resource, group, namespace string) (int, error) {
	cfg, err := restConfigFromKubeconfig(kubeconfigPath)
	if err != nil {
		return 0, err
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return 0, err
	}
	gvr, err := d.mapper.ResourceFor(schema.GroupVersionResource{Group: group, Resource: resource})
	if err != nil {
		return 0, err
	}
	var listErr error
	if namespace == "" {
		_, listErr = dyn.Resource(gvr).List(ctx, metav1.ListOptions{Limit: 1})
	} else {
		_, listErr = dyn.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{Limit: 1})
	}
	if listErr == nil {
		return 200, nil
	}
	if statusErr, ok := listErr.(apierrors.APIStatus); ok {
		return int(statusErr.Status().Code), nil
	}
	return 0, listErr
}

// --- harness helpers ---

func (d *KindDriver) SeedLimitedIdentity(ctx context.Context, name string, verbs, resources, resourceNames []string) error {
	if _, err := d.admin.CoreV1().ServiceAccounts(d.namespace).Create(ctx,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.namespace}},
		metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.namespace},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""}, Resources: resources, Verbs: verbs, ResourceNames: resourceNames,
		}},
	}
	if _, err := d.admin.RbacV1().Roles(d.namespace).Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: d.namespace}},
	}
	if _, err := d.admin.RbacV1().RoleBindings(d.namespace).Create(ctx, rb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return d.registerIdentityToken(ctx, name)
}

// SeedPartialCreatorIdentity seeds an operator that may read pods and create/delete
// ServiceAccounts and Roles, but may NOT create RoleBindings. A real mint run AS this
// identity therefore gets past the SA and Role and fails at the binding (403),
// exercising reverse-order rollback (FR-005). The seed objects carry no managed-by
// label, so they are invisible to CountManaged.
func (d *KindDriver) SeedPartialCreatorIdentity(ctx context.Context, name string) error {
	if _, err := d.admin.CoreV1().ServiceAccounts(d.namespace).Create(ctx,
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.namespace}},
		metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.namespace},
		Rules: []rbacv1.PolicyRule{
			// read pods so the SSAR pre-flight passes and creation is reached
			{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get", "list", "watch"}},
			// create + delete SAs and Roles so creation reaches the binding and rollback can clean up
			{APIGroups: []string{""}, Resources: []string{"serviceaccounts"}, Verbs: []string{"create", "delete"}},
			{APIGroups: []string{"rbac.authorization.k8s.io"}, Resources: []string{"roles"}, Verbs: []string{"create", "delete"}},
			// intentionally NO rolebindings verbs — the binding create returns 403
		},
	}
	if _, err := d.admin.RbacV1().Roles(d.namespace).Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.namespace},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: name},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: d.namespace}},
	}
	if _, err := d.admin.RbacV1().RoleBindings(d.namespace).Create(ctx, rb, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return d.registerIdentityToken(ctx, name)
}

// registerIdentityToken mints a 1h token for the ServiceAccount `name` in the active
// namespace, writes a kubeconfig for it, and registers it under d.identities[name].
func (d *KindDriver) registerIdentityToken(ctx context.Context, name string) error {
	tr, err := d.admin.CoreV1().ServiceAccounts(d.namespace).CreateToken(ctx, name,
		&authenticationv1.TokenRequest{Spec: authenticationv1.TokenRequestSpec{ExpirationSeconds: ptr.To(int64(3600))}},
		metav1.CreateOptions{})
	if err != nil {
		return err
	}
	path := filepath.Join(d.workDir, name+".kubeconfig")
	if err := d.writeIdentityKubeconfig(path, tr.Status.Token, d.namespace); err != nil {
		return err
	}
	d.identities[name] = path
	return nil
}

func (d *KindDriver) SeedUnmanagedRBAC(ctx context.Context, name string) error {
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: d.namespace}, // no managed-by label
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "view"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "default", Namespace: d.namespace}},
	}
	_, err := d.admin.RbacV1().RoleBindings(d.namespace).Create(ctx, rb, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (d *KindDriver) SeedManagedSession(ctx context.Context, sessionID string, expiresAt time.Time) error {
	om := metav1.ObjectMeta{
		Name:      "tessera-seed-" + sessionID,
		Namespace: d.namespace,
		Labels: map[string]string{
			managedByKey: managedByValue,
			sessionIDKey: sessionID,
			ownerKey:     "seed",
		},
		Annotations: map[string]string{expiresAtKey: expiresAt.UTC().Format(time.RFC3339)},
	}
	if _, err := d.admin.CoreV1().ServiceAccounts(d.namespace).Create(ctx,
		&corev1.ServiceAccount{ObjectMeta: om}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	role := &rbacv1.Role{ObjectMeta: om, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}}
	if _, err := d.admin.RbacV1().Roles(d.namespace).Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}
	return nil
}

func (d *KindDriver) KubeconfigFileExists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// --- lifecycle ---

func (d *KindDriver) EnsureNamespace(ctx context.Context, namespace string) error {
	_, err := d.admin.CoreV1().Namespaces().Create(ctx,
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (d *KindDriver) DeleteNamespace(ctx context.Context, namespace string) error {
	err := d.admin.CoreV1().Namespaces().Delete(ctx, namespace, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (d *KindDriver) DeleteSessionByLabel(ctx context.Context, sessionID string) error {
	sel := fmt.Sprintf("%s=%s,%s=%s", managedByKey, managedByValue, sessionIDKey, sessionID)
	opts := metav1.DeleteOptions{}
	lopts := metav1.ListOptions{LabelSelector: sel}
	_ = d.admin.RbacV1().ClusterRoleBindings().DeleteCollection(ctx, opts, lopts)
	_ = d.admin.RbacV1().ClusterRoles().DeleteCollection(ctx, opts, lopts)
	return nil
}

func (d *KindDriver) Close() {
	if d.workDir != "" {
		_ = os.RemoveAll(d.workDir)
	}
}

// --- helpers ---

func (d *KindDriver) writeIdentityKubeconfig(path, token, namespace string) error {
	caData := d.adminConfig.CAData
	if len(caData) == 0 && d.adminConfig.CAFile != "" {
		b, err := os.ReadFile(d.adminConfig.CAFile)
		if err != nil {
			return err
		}
		caData = b
	}
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["kind"] = &clientcmdapi.Cluster{
		Server:                   d.adminConfig.Host,
		CertificateAuthorityData: caData,
		InsecureSkipTLSVerify:    d.adminConfig.Insecure,
	}
	cfg.AuthInfos["limited"] = &clientcmdapi.AuthInfo{Token: token}
	cfg.Contexts["limited"] = &clientcmdapi.Context{Cluster: "kind", AuthInfo: "limited", Namespace: namespace}
	cfg.CurrentContext = "limited"
	return clientcmd.WriteToFile(*cfg, path)
}

func restConfigFromKubeconfig(path string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", path)
}

func clientsetFromKubeconfig(path string) (*kubernetes.Clientset, error) {
	cfg, err := restConfigFromKubeconfig(path)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(cfg)
}

func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}
