package main

// observeJavaFrame reports only validated framebuffer publication. Synthetic
// expected pixels and title-specific input checks belong to the corpus runner,
// so this is deliberately not a boots/playable milestone.
func observeJavaFrame(result *probeResult, slice uint64) bool {
	if result.Java == nil || !result.Java.Started || !result.Java.HasDisplay || result.Java.Instructions == 0 ||
		result.Java.PresentCount == 0 || !result.Java.FrameValid {
		return false
	}
	result.Status = "ok_frame"
	result.Level = "loads"
	result.FirstFrameSlice = slice
	return true
}
