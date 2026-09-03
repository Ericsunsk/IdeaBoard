package users

func NewUserService(
	repo *UserRepository,
	lifecycle UserLifecycle,
	creditBalanceReader UserCreditBalanceReader,
	uploadCleaner ...UserUploadCleaner,
) *UserService {
	var cleaner UserUploadCleaner
	if len(uploadCleaner) > 0 {
		cleaner = uploadCleaner[0]
	}
	return newUserService(repo, lifecycle, creditBalanceReader, cleaner, nil)
}
