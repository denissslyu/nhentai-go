package nhentai

import "math/rand"

var subDomainIList = []string{"i", "i2", "i3", "i5", "i7"}
var subDomainTList = []string{"t", "t2", "t3", "t5", "t7"}

func getRandomSubDomainI() string {
	return subDomainIList[rand.Intn(5)]
}

func getRandomSubDomainT() string {
	return subDomainTList[rand.Intn(5)]
}
