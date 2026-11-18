package app

func validateChannelSortOrder(value int) error {
	if value < 0 || value > 999999 {
		return BadAuthRequest("排序值必须是 0-999999 的整数，数值越小越靠前")
	}
	return nil
}
