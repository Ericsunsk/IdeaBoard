package http

import (
	"context"
	"fmt"
	"net/http"

	"ideaboard/api/internal/credits/domain"
	"ideaboard/api/internal/identity/principal"
	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	service      UserWalletService
	userResolver UserIDResolver
}

func NewUserHandler(service UserWalletService, resolver UserIDResolver) *UserHandler {
	return &UserHandler{service: service, userResolver: resolver}
}

func (h *UserHandler) GetCurrentWallet(c *gin.Context) {
	if h == nil || h.service == nil {
		response.InternalError(c, http.StatusServiceUnavailable, "Idea服务暂不可用", nil)
		return
	}
	userID, ok := authenticatedUserID(c)
	if !ok {
		return
	}
	wallet, err := h.service.GetWallet(c.Request.Context(), userID)
	if err != nil {
		writeCreditError(c, err)
		return
	}
	if err := enrichWallet(c.Request.Context(), h.userResolver, &wallet); err != nil {
		writeCreditError(c, err)
		return
	}
	response.SuccessDTO(c, wallet.WalletBalance)
}

func authenticatedUserID(c *gin.Context) (int64, bool) {
	identityPrincipal, ok := principal.Require(c)
	if !ok {
		return 0, false
	}
	return identityPrincipal.UserID(), true
}

func enrichWallet(ctx context.Context, userResolver UserIDResolver, wallet *domain.WalletDetails) error {
	if userResolver == nil || wallet == nil || wallet.UserID <= 0 {
		return nil
	}
	user, err := userResolver.ResolveID(ctx, wallet.UserID)
	if err != nil {
		return fmt.Errorf("resolve credit wallet identity: %w", err)
	}
	wallet.PublicID = user.PublicID
	wallet.DisplayName = user.DisplayName
	for index := range wallet.ActiveRestrictions {
		wallet.ActiveRestrictions[index].PublicID = user.PublicID
		wallet.ActiveRestrictions[index].DisplayName = user.DisplayName
	}
	return nil
}
