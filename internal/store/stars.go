package store

import (
	"context"
	"time"
)

// SetFileStarred añade o retira un archivo de Destacados para un usuario.
func (s *Store) SetFileStarred(ctx context.Context, userID, fileID string, starred bool) error {
	if userID == "" || fileID == "" {
		return ErrNotFound
	}
	return s.mutate(ctx, func(next *persistedState) error {
		userExists := false
		for _, user := range next.Users {
			if user.ID == userID {
				userExists = true
				break
			}
		}
		if !userExists {
			return ErrNotFound
		}
		filtered := next.Stars[:0]
		found := false
		for _, item := range next.Stars {
			if item.UserID == userID && item.FileID == fileID {
				found = true
				if starred {
					filtered = append(filtered, item)
				}
				continue
			}
			filtered = append(filtered, item)
		}
		next.Stars = filtered
		if starred && !found {
			next.Stars = append(next.Stars, FileStar{UserID: userID, FileID: fileID, CreatedAt: time.Now().UTC()})
		}
		return nil
	})
}

func (s *Store) FileStarred(ctx context.Context, userID, fileID string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, item := range s.state.Stars {
		if item.UserID == userID && item.FileID == fileID {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) StarredFileIDs(ctx context.Context, userID string) (map[string]struct{}, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]struct{})
	for _, item := range s.state.Stars {
		if item.UserID == userID {
			out[item.FileID] = struct{}{}
		}
	}
	return out, nil
}

// MoveStarredFileID mantiene Destacados cuando una operación cambia el ID estable
// del catálogo (renombrar o mover dentro/entre unidades).
func (s *Store) MoveStarredFileID(ctx context.Context, oldID, newID string) error {
	if oldID == "" || newID == "" || oldID == newID {
		return nil
	}
	return s.mutate(ctx, func(next *persistedState) error {
		seen := make(map[string]struct{}, len(next.Stars))
		out := next.Stars[:0]
		for _, item := range next.Stars {
			if item.FileID == oldID {
				item.FileID = newID
			}
			key := item.UserID + "\x00" + item.FileID
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, item)
		}
		next.Stars = out
		return nil
	})
}

func (s *Store) DeleteStarredFileIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			set[id] = struct{}{}
		}
	}
	return s.mutate(ctx, func(next *persistedState) error {
		out := next.Stars[:0]
		for _, item := range next.Stars {
			if _, remove := set[item.FileID]; !remove {
				out = append(out, item)
			}
		}
		next.Stars = out
		return nil
	})
}
