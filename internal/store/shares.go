package store

import (
	"context"
	"time"
)

// PublicShare representa un enlace público revocable para un archivo del catálogo.
// El token es aleatorio y funciona como secreto portador de la URL pública.
type PublicShare struct {
	ID           string    `json:"id"`
	OwnerUserID  string    `json:"owner_user_id"`
	FileID       string    `json:"file_id"`
	Token        string    `json:"token"`
	PasswordHash string    `json:"password_hash,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	LastAccessAt time.Time `json:"last_access_at,omitempty"`
	AccessCount  uint64    `json:"access_count,omitempty"`
}

func (s *Store) UpsertPublicShare(ctx context.Context, ownerUserID, fileID, token, passwordHash string) (PublicShare, error) {
	if err := ctx.Err(); err != nil {
		return PublicShare{}, err
	}
	if ownerUserID == "" || fileID == "" || token == "" {
		return PublicShare{}, ErrNotFound
	}
	var out PublicShare
	err := s.mutate(ctx, func(next *persistedState) error {
		userExists := false
		for _, user := range next.Users {
			if user.ID == ownerUserID {
				userExists = true
				break
			}
		}
		if !userExists {
			return ErrNotFound
		}
		now := time.Now().UTC()
		for i := range next.Shares {
			if next.Shares[i].OwnerUserID == ownerUserID && next.Shares[i].FileID == fileID {
				next.Shares[i].PasswordHash = passwordHash
				next.Shares[i].UpdatedAt = now
				out = next.Shares[i]
				return nil
			}
		}
		id, err := randomID(16)
		if err != nil {
			return err
		}
		out = PublicShare{ID: id, OwnerUserID: ownerUserID, FileID: fileID, Token: token, PasswordHash: passwordHash, CreatedAt: now, UpdatedAt: now}
		next.Shares = append(next.Shares, out)
		return nil
	})
	return out, err
}

func (s *Store) PublicShareByToken(ctx context.Context, token string) (PublicShare, error) {
	if err := ctx.Err(); err != nil {
		return PublicShare{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, share := range s.state.Shares {
		if share.Token == token {
			return share, nil
		}
	}
	return PublicShare{}, ErrNotFound
}

func (s *Store) PublicShareByID(ctx context.Context, id string) (PublicShare, error) {
	if err := ctx.Err(); err != nil {
		return PublicShare{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, share := range s.state.Shares {
		if share.ID == id {
			return share, nil
		}
	}
	return PublicShare{}, ErrNotFound
}

func (s *Store) PublicShareByFile(ctx context.Context, ownerUserID, fileID string) (PublicShare, error) {
	if err := ctx.Err(); err != nil {
		return PublicShare{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, share := range s.state.Shares {
		if share.OwnerUserID == ownerUserID && share.FileID == fileID {
			return share, nil
		}
	}
	return PublicShare{}, ErrNotFound
}

func (s *Store) PublicShares(ctx context.Context) ([]PublicShare, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := append([]PublicShare(nil), s.state.Shares...)
	return out, nil
}

func (s *Store) RenewPublicShare(ctx context.Context, id, token string) (PublicShare, error) {
	if id == "" || token == "" {
		return PublicShare{}, ErrNotFound
	}
	var out PublicShare
	err := s.mutate(ctx, func(next *persistedState) error {
		for i := range next.Shares {
			if next.Shares[i].ID == id {
				next.Shares[i].Token = token
				next.Shares[i].UpdatedAt = time.Now().UTC()
				out = next.Shares[i]
				return nil
			}
		}
		return ErrNotFound
	})
	return out, err
}

func (s *Store) SetPublicSharePassword(ctx context.Context, id, passwordHash string) (PublicShare, error) {
	var out PublicShare
	err := s.mutate(ctx, func(next *persistedState) error {
		for i := range next.Shares {
			if next.Shares[i].ID == id {
				next.Shares[i].PasswordHash = passwordHash
				next.Shares[i].UpdatedAt = time.Now().UTC()
				out = next.Shares[i]
				return nil
			}
		}
		return ErrNotFound
	})
	return out, err
}

func (s *Store) TouchPublicShare(ctx context.Context, id string, when time.Time) error {
	return s.mutate(ctx, func(next *persistedState) error {
		for i := range next.Shares {
			if next.Shares[i].ID == id {
				next.Shares[i].LastAccessAt = when.UTC()
				next.Shares[i].AccessCount++
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) DeletePublicShare(ctx context.Context, id string) error {
	return s.mutate(ctx, func(next *persistedState) error {
		out := next.Shares[:0]
		found := false
		for _, share := range next.Shares {
			if share.ID == id {
				found = true
				continue
			}
			out = append(out, share)
		}
		if !found {
			return ErrNotFound
		}
		next.Shares = out
		return nil
	})
}

func (s *Store) DeletePublicSharesByOwner(ctx context.Context, ownerUserID string) (int, error) {
	removed := 0
	err := s.mutate(ctx, func(next *persistedState) error {
		out := next.Shares[:0]
		for _, share := range next.Shares {
			if ownerUserID == "" || share.OwnerUserID == ownerUserID {
				removed++
				continue
			}
			out = append(out, share)
		}
		next.Shares = out
		return nil
	})
	return removed, err
}

func (s *Store) DeletePublicSharesByFileIDs(ctx context.Context, fileIDs []string) error {
	if len(fileIDs) == 0 {
		return nil
	}
	ids := make(map[string]struct{}, len(fileIDs))
	for _, id := range fileIDs {
		if id != "" {
			ids[id] = struct{}{}
		}
	}
	return s.mutate(ctx, func(next *persistedState) error {
		out := next.Shares[:0]
		for _, share := range next.Shares {
			if _, remove := ids[share.FileID]; remove {
				continue
			}
			out = append(out, share)
		}
		next.Shares = out
		return nil
	})
}

func (s *Store) MovePublicShareFileID(ctx context.Context, oldID, newID string) error {
	if oldID == "" || newID == "" || oldID == newID {
		return nil
	}
	return s.mutate(ctx, func(next *persistedState) error {
		for i := range next.Shares {
			if next.Shares[i].FileID == oldID {
				next.Shares[i].FileID = newID
				next.Shares[i].UpdatedAt = time.Now().UTC()
			}
		}
		return nil
	})
}
