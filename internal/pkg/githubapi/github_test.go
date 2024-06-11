package githubapi

import "testing"

func TestGenerateSafePromotionBranchName(t *testing.T) {
	t.Parallel()
	prNumber := 11
	originBranch := "originBranch"
	targetPaths := []string{"targetPath1", "targetPath2"}
	result := generateSafePromotionBranchName(prNumber, originBranch, targetPaths)
	expectedResult := "promotions/11-originBranch-676f02019f18"
	if result != expectedResult {
		t.Errorf("Expected %s, got %s", expectedResult, result)
	}
}

// TestGenerateSafePromotionBranchNameLongBranchName tests the case where the original  branch name is longer than 250 characters
func TestGenerateSafePromotionBranchNameLongBranchName(t *testing.T) {
	t.Parallel()
	prNumber := 11
	originBranch := "ooriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchoriginBranchriginBranch"
	targetPaths := []string{"targetPath1", "targetPath2"}
	result := generateSafePromotionBranchName(prNumber, originBranch, targetPaths)
	if len(result) > 250 {
		t.Errorf("Expected branch name to be less than 250 characters, got %d", len(result))
	}
}

// TestGenerateSafePromotionBranchNameLongTargets tests the case where the target paths are longer than 250 characters
func TestGenerateSafePromotionBranchNameLongTargets(t *testing.T) {
	t.Parallel()
	prNumber := 11
	originBranch := "originBranch"
	targetPaths := []string{
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath1",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath2",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath3",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath4",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath5",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath6",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath7",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath8",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath9",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath10",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath11",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath12",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath13",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath14",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath15",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath16",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath17",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath18",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath19",
		"loooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooooongtargetPath20",
	}
	result := generateSafePromotionBranchName(prNumber, originBranch, targetPaths)
	if len(result) > 250 {
		t.Errorf("Expected branch name to be less than 250 characters, got %d", len(result))
	}
}
