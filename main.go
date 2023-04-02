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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
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
	accessKeyIDPattern := regexp.MustCompile(`(?i)(AWS_ACCESS_KEY_ID|aws_access_key_id)[=:]["']?([\w\/\+]+)["']?`)
	secretAccessKeyPattern := regexp.MustCompile(`(?i)(AWS_SECRET_ACCESS_KEY|aws_secret_access_key)[=:]["']?([^ \t\r\n\v\f]+)["']?`)

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

	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("us-west-2")},
	)

	if err != nil {
		return false
	}

	svc := iam.New(sess)

	result, err := svc.GetAccessKeyLastUsed(&iam.GetAccessKeyLastUsedInput{
		AccessKeyId: aws.String(accessKeyID),
	})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok {
			switch awsErr.Code() {
			case iam.ErrCodeNoSuchEntityException:
				return false
			default:
				return false
			}
		} else {
			return false
		}
	}

	if result != nil && result.UserName != nil {
		return true
	} else {
		return false
	}
}

func main() {
	// Parse command line arguments
	repoURL := flag.String("repo", "", "GitHub repository URL")
	flag.Parse()

	if *repoURL == "" {
		log.Fatal("Please provide a GitHub repository URL using the -repo flag.")
	}

	// Start the timer
	startTime := time.Now()

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

	// Create a channel to communicate errors from goroutines
	errChan := make(chan error, 1)

	// Iterate over commit hashes and spawn a goroutine to search for IAM keys in each commit
	for _, commitHash := range commitHashes {
		go func(commitHash string) {
			// Checkout the commit
			err := checkoutCommit(repoPath, commitHash)
			if err != nil {
				errChan <- fmt.Errorf("error checking out commit %s: %v", commitHash, err)
				return
			}

			// Search for IAM keys in the repository
			foundIAMKeys, err := searchIAMKeysInRepo(repoPath)
			if err != nil {
				errChan <- fmt.Errorf("error searching for IAM keys in commit %s: %v", commitHash, err)
				return
			}

			// Spawn a goroutine for each IAM key found in the repository to validate the key concurrently
			for _, iamKeys := range foundIAMKeys {
				for accessKeyID, secretAccessKey := range iamKeys {
					fmt.Println(validateIAMKey(accessKeyID, secretAccessKey))
					go func(accessKeyID, secretAccessKey string) {
						if valid := validateIAMKey(accessKeyID, secretAccessKey); valid {
							validKeysFound = true
							fmt.Printf("Valid IAM key found in commit %s: %s\n", commitHash, accessKeyID)
						}
					}(accessKeyID, secretAccessKey)
				}
			}
		}(commitHash)
	}

	// Wait for all goroutines to finish
	for i := 0; i < len(commitHashes); i++ {
		select {
		case err := <-errChan:
			log.Fatalf("%v", err)
		default:
			// No errors, continue
		}
	}

	if !validKeysFound {
		fmt.Println("\nNo valid IAM keys found in the repository.")
	}

	duration := time.Since(startTime).Round(time.Second / 100).String()

	fmt.Printf("\nTotal time taken: %v\n", duration)
}
