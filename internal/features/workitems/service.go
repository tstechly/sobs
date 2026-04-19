package workitems

import (
	"context"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Item struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

type Service struct {
	storeFactory extensionpoints.StoreFactory
}

func NewService() *Service {
	return &Service{}
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) List() []Item {
	if s.storeFactory != nil {
		store, err := persist.Open(context.Background(), s.storeFactory)
		if err == nil {
			defer func() { _ = store.Close() }()
			rows, err := store.Query(context.Background(), "SELECT Id, IssueTitle, if(IssueState = '', 'open', IssueState) FROM sobs_github_work_items FINAL WHERE IsDeleted = 0 ORDER BY CreatedAt DESC LIMIT 100")
			if err == nil {
				defer func() { _ = rows.Close() }()
				out := []Item{}
				for rows.Next() {
					var item Item
					if err := rows.Scan(&item.ID, &item.Title, &item.Status); err != nil {
						return out
					}
					out = append(out, item)
				}
				return out
			}
		}
	}
	return []Item{{ID: "wi-1", Title: "Migration tracker", Status: "open"}}
}
