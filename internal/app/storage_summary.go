package app

import (
	"context"
	"path"
	"sort"
	"strings"
	"time"

	storagepkg "personalcloud/internal/storage"
)

func storageUsageFromViews(views []storagepkg.View) (storageSummary, []storageUsageItem) {
	var summary storageSummary
	items := make([]storageUsageItem, 0, len(views))
	for _, view := range views {
		if !view.Registered || !view.Online || view.Capacity == 0 {
			continue
		}
		free := view.Free
		if free > view.Capacity {
			free = view.Capacity
		}
		used := view.Capacity - free
		summary.Total += view.Capacity
		summary.Free += free
		summary.Used += used
		summary.OnlineUnits++
		items = append(items, storageUsageItem{
			ID:          view.ID,
			Name:        view.Name,
			VirtualRoot: view.VirtualRoot,
			FSType:      view.FSType,
			MountPoint:  view.MountPoint,
			Capacity:    view.Capacity,
			Used:        used,
			Free:        free,
			PercentUsed: percentageUsed(used, view.Capacity),
			ReadOnly:    view.ReadOnly,
			Mounted:     view.Mounted,
			Online:      view.Online,
		})
	}
	summary.PercentUsed = percentageUsed(summary.Used, summary.Total)
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
	return summary, items
}

func percentageUsed(used, total uint64) int {
	if total == 0 {
		return 0
	}
	if used > total {
		used = total
	}
	return int((float64(used)/float64(total))*100 + 0.5)
}

func (a *App) storageSummaryForContext(ctx context.Context) storageSummary {
	if a.storageManager == nil {
		return storageSummary{}
	}
	views, err := a.storageManager.Views(ctx)
	if err != nil {
		return storageSummary{}
	}
	summary, _ := storageUsageFromViews(views)
	return summary
}

func (a *App) storageLargestOnlineFiles(ctx context.Context, userID string, views []storagepkg.View, filter explorerFilter, limit int) []explorerItem {
	if limit <= 0 {
		return nil
	}
	online := make(map[string]storagepkg.View, len(views))
	for _, view := range views {
		if view.Registered && view.Online {
			online[view.ID] = view
		}
	}
	stars := map[string]struct{}{}
	if userID != "" {
		stars, _ = a.store.StarredFileIDs(ctx, userID)
	}
	files := a.catalog.AllFiles()
	items := make([]explorerItem, 0, len(files))
	for _, file := range files {
		view, ok := online[file.StorageID]
		if !ok {
			continue
		}
		_, starred := stars[file.ID]
		location := path.Dir(file.RelativePath)
		if location == "." || location == "/" {
			location = "/" + file.VirtualRoot
		} else {
			location = path.Join("/", file.VirtualRoot, location)
		}
		item := explorerItem{
			ID:          file.ID,
			Name:        file.Name,
			Kind:        file.Kind,
			Size:        file.Size,
			ModTime:     file.ModTime,
			URL:         "/archivo/" + file.ID + "/original",
			DownloadURL: "/archivo/" + file.ID + "/original",
			Location:    location,
			Offline:     false,
			StorageName: view.Name,
			VirtualRoot: file.VirtualRoot,
			Health:      file.Health,
			Starred:     starred,
		}
		if file.Thumbnail {
			item.ThumbnailURL = catalogCacheURL(file, "miniatura")
		}
		decorateExplorerFile(&item)
		items = append(items, item)
	}
	items = applyExplorerFilter(items, filter, time.Now().UTC())
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Size == items[j].Size {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].Size > items[j].Size
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}
