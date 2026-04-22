package assets

import (
			"strings"
)

type StyleCardService struct {
	repo *StyleCardRepository
}

func NewStyleCardService(repo *StyleCardRepository) *StyleCardService {
	return &StyleCardService{repo: repo}
}

func (s *StyleCardService) Create(req *CreateStyleCardRequest) (*StyleCard, error) {
	card := &StyleCard{
		Name:        req.Name,
		CoverImage:  req.CoverImage,
		Keywords:    req.Keywords,
		SortOrder:   req.SortOrder,
		IsEnabled:   req.IsEnabled,
		IsExclusive: req.IsExclusive,
	}
	if err := s.repo.Create(card); err != nil {
		return nil, err
	}
	return card, nil
}

func (s *StyleCardService) Update(id uint64, req *UpdateStyleCardRequest) error {
	card, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	card.Name = req.Name
	card.CoverImage = req.CoverImage
	card.Keywords = req.Keywords
	card.SortOrder = req.SortOrder
	card.IsEnabled = req.IsEnabled
	card.IsExclusive = req.IsExclusive
	return s.repo.Update(card)
}

func (s *StyleCardService) Delete(id uint64) error {
	return s.repo.Delete(id)
}

func (s *StyleCardService) GetByID(id uint64) (*StyleCard, error) {
	return s.repo.GetByID(id)
}

func (s *StyleCardService) List(req *StyleCardListRequest) ([]*StyleCard, int64, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}
	return s.repo.List(req.Page, req.PageSize)
}

func (s *StyleCardService) ListEnabled() ([]*StyleCard, error) {
	return s.repo.ListEnabled()
}

// BuildPrompt 拼接完整提示词
// userPrompt: 用户输入的描述
// cardIDs: 用户选中的风格卡片 ID 列表
// 返回完整提示词字符串
func (s *StyleCardService) BuildPrompt(userPrompt string, cardIDs []uint64) (string, error) {
	if len(cardIDs) == 0 && userPrompt == "" {
		return "", nil
	}

	var keywords []string

	if len(cardIDs) > 0 {
		cards, err := s.repo.GetByIDs(cardIDs)
		if err != nil {
			return "", err
		}

		for _, card := range cards {
			// 按逗号分隔关键词
			parts := strings.Split(card.Keywords, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					keywords = append(keywords, part)
				}
			}
		}
	}

	// 拼接：关键词在前，用户描述在后
	var fullPrompt string
	if len(keywords) > 0 {
		fullPrompt = strings.Join(keywords, ", ") + ". " + userPrompt
	} else {
		fullPrompt = userPrompt
	}

	return fullPrompt, nil
}
