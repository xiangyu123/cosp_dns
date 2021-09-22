package whitedomain

func GetAllDomains() []string {
	// 元素去重
	domain := removeRepByLoop(whiteDomains)
	return domain
}

func removeRepByLoop(slc []string) []string {
    result := []string{}  // 存放结果
    for i := range slc{
        flag := true
        for j := range result{
            if slc[i] == result[j] {
                flag = false  // 存在重复元素，标识为false
                break
            }
        }
        if flag {  // 标识为false，不添加进结果
            result = append(result, slc[i])
        }
    }
    return result
}