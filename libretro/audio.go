package libretro

func stereoPCM(input []int16, channels int) []int16 {
	switch channels {
	case 1:
		output := make([]int16, len(input)*2)
		for index, sample := range input {
			output[index*2] = sample
			output[index*2+1] = sample
		}
		return output
	case 2:
		return input
	default:
		return nil
	}
}
