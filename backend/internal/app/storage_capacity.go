package app

// Temporary media processing uses the Studio engineering limit. Platform
// validates the account capacity when material uploads are authorized.
func (s *Service) storedFileLimit(_ string, policy RuntimePolicySetting) (int64, error) {
	return gigabytes(policy.Resource.StoredFileGB), nil
}
