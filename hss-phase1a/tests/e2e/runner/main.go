package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type TierResult struct {
	Name   string
	Passed int
	Failed int
	Total  int
	Status string
}

func main() {
	fmt.Println("================================================================================")
	fmt.Println("   PULSE SURVEILLANCE PLATFORM — YOLO11 FIRE & SMOKE E2E TEST RUNNER")
	fmt.Println("   4-Tier Requirements-Driven Verification Engine")
	fmt.Println("================================================================================")
	fmt.Printf("Started at: %s\n\n", time.Now().Format(time.RFC3339))

	cmd := exec.Command("go", "test", "-v", "./tests/e2e/...")
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	startTime := time.Now()
	err := cmd.Run()
	duration := time.Since(startTime)

	output := stdoutBuf.String() + "\n" + stderrBuf.String()
	scanner := bufio.NewScanner(strings.NewReader(output))

	tier1Passed, tier1Failed := 0, 0
	tier2Passed, tier2Failed := 0, 0
	tier3Passed, tier3Failed := 0, 0
	tier4Passed, tier4Failed := 0, 0

	passRegex := regexp.MustCompile(`--- PASS:\s+(Test\w+)`)
	failRegex := regexp.MustCompile(`--- FAIL:\s+(Test\w+)`)

	for scanner.Scan() {
		line := scanner.Text()

		if matches := passRegex.FindStringSubmatch(line); len(matches) > 1 {
			testName := matches[1]
			if strings.Contains(testName, "TestTier1") {
				tier1Passed++
			} else if strings.Contains(testName, "TestTier2") {
				tier2Passed++
			} else if strings.Contains(testName, "TestTier3") {
				tier3Passed++
			} else if strings.Contains(testName, "TestTier4") {
				tier4Passed++
			}
		}

		if matches := failRegex.FindStringSubmatch(line); len(matches) > 1 {
			testName := matches[1]
			if strings.Contains(testName, "TestTier1") {
				tier1Failed++
			} else if strings.Contains(testName, "TestTier2") {
				tier2Failed++
			} else if strings.Contains(testName, "TestTier3") {
				tier3Failed++
			} else if strings.Contains(testName, "TestTier4") {
				tier4Failed++
			}
		}
	}

	totalPassed := tier1Passed + tier2Passed + tier3Passed + tier4Passed
	totalFailed := tier1Failed + tier2Failed + tier3Failed + tier4Failed
	totalTests := totalPassed + totalFailed

	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("%-35s | %-8s | %-8s | %-8s | %-8s\n", "Test Tier", "Passed", "Failed", "Total", "Status")
	fmt.Println("--------------------------------------------------------------------------------")

	printTier("Tier 1: Feature Coverage (F1..F8)", tier1Passed, tier1Failed)
	printTier("Tier 2: Boundary & Corner Cases", tier2Passed, tier2Failed)
	printTier("Tier 3: Pairwise Cross-Feature", tier3Passed, tier3Failed)
	printTier("Tier 4: Real-World Scenarios", tier4Passed, tier4Failed)

	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Printf("%-35s | %-8d | %-8d | %-8d | %-8s\n", "TOTAL E2E VERIFICATION SUITE", totalPassed, totalFailed, totalTests, getStatus(totalFailed))
	fmt.Println("================================================================================")
	fmt.Printf("Execution Completed in: %v\n", duration)

	if err != nil || totalFailed > 0 || totalTests == 0 {
		fmt.Printf("\n[FAILURE] Test suite finished with %d failures.\n", totalFailed)
		os.Exit(1)
	}

	fmt.Printf("\n[SUCCESS] 100%% of E2E verification tests passed with ZERO defects!\n")
	os.Exit(0)
}

func printTier(name string, passed, failed int) {
	total := passed + failed
	status := getStatus(failed)
	fmt.Printf("%-35s | %-8d | %-8d | %-8d | %-8s\n", name, passed, failed, total, status)
}

func getStatus(failed int) string {
	if failed > 0 {
		return "FAIL"
	}
	return "PASS"
}
