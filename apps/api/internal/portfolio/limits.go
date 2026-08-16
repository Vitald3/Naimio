package portfolio

import "context"

type LimitPolicy interface {
	Limit(context.Context, string, string) (value int64, unlimited bool, enforced bool, err error)
}
type LimitedRepository struct {
	Repository Repository
	Policy     LimitPolicy
}

func (r LimitedRepository) allows(ctx context.Context, actor, key string, current, incoming int) error {
	if r.Policy == nil {
		return nil
	}
	limit, unlimited, enforced, err := r.Policy.Limit(ctx, actor, key)
	if err != nil {
		return err
	}
	if !enforced || unlimited {
		return nil
	}
	if int64(current+incoming) > limit {
		if key == "portfolio.media_limit" {
			return ErrMediaLimit
		}
		return ErrItemLimit
	}
	return nil
}
func (r LimitedRepository) Create(ctx context.Context, actor string, in WriteRequest) (Item, error) {
	items, err := r.Repository.ListOwned(ctx, actor)
	if err != nil {
		return Item{}, err
	}
	if err = r.allows(ctx, actor, "portfolio.item_limit", len(items), 1); err != nil {
		return Item{}, err
	}
	if err = r.allows(ctx, actor, "portfolio.media_limit", 0, len(in.MediaObjectIDs)); err != nil {
		return Item{}, err
	}
	return r.Repository.Create(ctx, actor, in)
}
func (r LimitedRepository) GetOwned(ctx context.Context, a, id string) (Item, error) {
	return r.Repository.GetOwned(ctx, a, id)
}
func (r LimitedRepository) Update(ctx context.Context, a, id string, in WriteRequest) (Item, error) {
	if err := r.allows(ctx, a, "portfolio.media_limit", 0, len(in.MediaObjectIDs)); err != nil {
		return Item{}, err
	}
	return r.Repository.Update(ctx, a, id, in)
}
func (r LimitedRepository) Delete(ctx context.Context, a, id string) error {
	return r.Repository.Delete(ctx, a, id)
}
func (r LimitedRepository) AttachMedia(ctx context.Context, a, id, media string, sort int) (Item, error) {
	item, err := r.Repository.GetOwned(ctx, a, id)
	if err != nil {
		return Item{}, err
	}
	for _, v := range item.Media {
		if v.ID == media {
			return r.Repository.AttachMedia(ctx, a, id, media, sort)
		}
	}
	if err = r.allows(ctx, a, "portfolio.media_limit", len(item.Media), 1); err != nil {
		return Item{}, err
	}
	return r.Repository.AttachMedia(ctx, a, id, media, sort)
}
func (r LimitedRepository) DetachMedia(ctx context.Context, a, id, media string) error {
	return r.Repository.DetachMedia(ctx, a, id, media)
}
func (r LimitedRepository) ListOwned(ctx context.Context, a string) ([]Item, error) {
	return r.Repository.ListOwned(ctx, a)
}
func (r LimitedRepository) ListPublic(ctx context.Context, u string, c *Cursor, l int) (Page, error) {
	return r.Repository.ListPublic(ctx, u, c, l)
}
