package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sts"
)

// cloneRepo clones the repository from the given URL and returns the local path to the cloned repository.
func cloneRepo(url string) (string, error) {
	// Create a temporary directory to store the cloned repository
	tempDir, err := ioutil.TempDir("", "repo-clone-")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %v", err)
	}

	// Run the git clone command
	cmd := exec.Command("git", "clone", url, tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to clone repository: %v. Output: %s", err, string(output))
	}

	return tempDir, nil
}

// getCommitHashes retrieves the commit hashes from the given repository path and returns them as a slice of strings.
func getCommitHashes(repoPath string) ([]string, error) {
	// Change working directory to the repository path
	err := os.Chdir(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to change working directory: %v", err)
	}

	// Run the git log command to get commit hashes
	cmd := exec.Command("git", "log", "--pretty=format:%H")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to get commit hashes: %v. Output: %s", err, string(output))
	}

	// Split the output by newline and return the commit hashes as a slice
	commitHashes := strings.Split(string(output), "\n")

	return commitHashes, nil
}

// checkoutCommit checks out the specified commit in the repository at the given path.
func checkoutCommit(repoPath, commitHash string) error {
	// Change working directory to the repository path
	err := os.Chdir(repoPath)
	if err != nil {
		return fmt.Errorf("failed to change working directory: %v", err)
	}

	// Run the git checkout command to switch to the specified commit
	cmd := exec.Command("git", "checkout", commitHash)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to checkout commit: %v. Output: %s", err, string(output))
	}

	return nil
}

// searchIAMKeysInFile searches for AWS IAM keys in the specified file and returns a map with access keys as keys and secret access keys as values.
func searchIAMKeysInFile(filePath string) (map[string]string, error) {
	// Read the file content
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	// Regular expressions to match Access Key ID and Secret Access Key
	accessKeyIDPattern := regexp.MustCompile(`AWS_ACCESS_KEY_ID=([A-Z0-9]+)`)
	secretAccessKeyPattern := regexp.MustCompile(`(?i)AWS_SECRET_ACCESS_KEY=([^ \t\r\n\v\f]+)`)

	// Find matches in the file content
	accessKeyIDs := accessKeyIDPattern.FindAllStringSubmatch(string(content), -1)
	secretAccessKeys := secretAccessKeyPattern.FindAllStringSubmatch(string(content), -1)

	// Combine the matched keys
	iamKeys := make(map[string]string)
	for _, match := range accessKeyIDs {
		accessKeyID := match[1]
		for _, secretMatch := range secretAccessKeys {
			if secretMatch[0] != "" {
				secretAccessKey := secretMatch[1]
				iamKeys[accessKeyID] = secretAccessKey
			}
		}
	}

	return iamKeys, nil
}

// searchIAMKeysInRepo searches for AWS IAM keys in the repository at the given path and returns a map of file paths to matched keys.
func searchIAMKeysInRepo(repoPath string) (map[string]map[string]string, error) {
	foundIAMKeys := make(map[string]map[string]string)

	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Search for IAM keys in the file
		iamKeys, err := searchIAMKeysInFile(path)
		if err != nil {
			return fmt.Errorf("failed to search IAM keys in file: %v", err)
		}

		// Add the matched keys to the map
		if len(iamKeys) > 0 {
			foundIAMKeys[path] = iamKeys
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to search IAM keys in repository: %v", err)
	}

	return foundIAMKeys, nil
}

func validateIAMKey(accessKeyID string, secretAccessKey string) bool {
	creds := credentials.NewStaticCredentials(accessKeyID, secretAccessKey, "")

	sess, err := session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String("ap-south-1"),
	})

	if err != nil {
		return false
	}

	stsSvc := sts.New(sess)
	_, err = stsSvc.GetCallerIdentity(&sts.GetCallerIdentityInput{})

	if err != nil {
		return false
	}

	return true
}

func main() {
	// Parse command line arguments
	repoURL := flag.String("repo", "", "GitHub repository URL")
	flag.Parse()

	if *repoURL == "" {
		log.Fatal("Please provide a GitHub repository URL using the -repo flag.")
	}

	// Clone the repository
	repoPath, err := cloneRepo(*repoURL)
	if err != nil {
		log.Fatalf("Error cloning repository: %v", err)
	}

	// Get commit hashes
	commitHashes, err := getCommitHashes(repoPath)
	if err != nil {
		log.Fatalf("Error getting commit hashes: %v", err)
	}

	validKeysFound := false

	for _, commitHash := range commitHashes {
		// Checkout the commit
		err := checkoutCommit(repoPath, commitHash)
		if err != nil {
			log.Fatalf("Error checking out commit %s: %v", commitHash, err)
		}

		// Search for IAM keys in the repository
		foundIAMKeys, err := searchIAMKeysInRepo(repoPath)
		if err != nil {
			log.Fatalf("Error searching for IAM keys: %v", err)
		}

		// Validate IAM keys
		for _, iamKeys := range foundIAMKeys {
			for accessKeyID, secretAccessKey := range iamKeys {
				if valid := validateIAMKey(accessKeyID, secretAccessKey); valid {
					validKeysFound = true
					fmt.Printf("Valid IAM key found in commit %s: %s\n", commitHash, accessKeyID)
				}
			}
		}

	}

	if !validKeysFound {
		fmt.Println("\nNo valid IAM keys found in the repository.")
	}
}
