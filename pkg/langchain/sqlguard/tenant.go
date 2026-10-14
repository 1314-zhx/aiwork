package sqlguard

import "context"

type tenantKey struct{}

// Tenant is the row-level identity attached by the application, never by the model.
type Tenant struct {
	UserID   string
	UserName string
	DeptIDs  []string
	IsSystem bool
}

func WithTenant(ctx context.Context, t Tenant) context.Context {
	return context.WithValue(ctx, tenantKey{}, t)
}

func TenantFrom(ctx context.Context) Tenant {
	if ctx == nil {
		return Tenant{}
	}
	t, _ := ctx.Value(tenantKey{}).(Tenant)
	return t
}

func (t Tenant) Skip() bool {
	// Logged-in users always carry an id; only empty system tenants skip injection (tests).
	return t.IsSystem && t.UserID == "" && t.UserName == ""
}

func (t Tenant) Values() []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(v string) {
		if v == "" {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	add(t.UserName)
	add(t.UserID)
	return out
}
