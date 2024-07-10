package middleware

import "context"

type contextKey string

const xidKey contextKey = "xid"

func SetXID(ctx context.Context, xid string) context.Context {
	return context.WithValue(ctx, xidKey, xid)
}

func GetXID(ctx context.Context) string {
	if xid, ok := ctx.Value(xidKey).(string); ok {
		return xid
	}
	return "-"
}
