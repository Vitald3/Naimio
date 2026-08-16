package monetization

import "context"

type LimitPolicy struct{ Service Service }

func (p LimitPolicy) Limit(ctx context.Context, user, key string) (int64, bool, bool, error) {
	caps, err := p.Service.Resolve(ctx, user)
	if err != nil {
		return 0, false, false, err
	}
	if !caps.ProSystemEnabled {
		return 0, true, false, nil
	}
	value, unlimited, ok := caps.Limit(key)
	return value, unlimited, ok, nil
}
