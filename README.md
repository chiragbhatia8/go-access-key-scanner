## Get Started

- Clone the repository to your local machine.
- Open a terminal and navigate to the directory where the code is located.
- Run the following command to build the binary file: go build -o aws-iam-keys-finder
- Run the binary file with the repository URL as an argument: ```./aws-iam-keys-finder https://github.com/username/repo.git```

Here's an example command to run the code:
```
./aws-iam-keys-finder https://github.com/aws-samples/aws-go-web-api.git
```

This will clone the repository at the specified URL, search for valid AWS IAM keys in the code, validate them, and report the valid keys found.

## Technical Documentation

The solution consists of a Golang program called aws-iam-keys-finder that takes a single argument, which is the URL of the GitHub repository to be scanned for valid AWS IAM keys. The program follows the following steps to accomplish this:

- Clone the repository locally using the cloneRepo function, which uses the os/exec package to run the git clone command.

- Get the list of commit hashes for the repository using the getCommitHashes function, which uses the os/exec package to run the git log --pretty=format:"%H" command.

- Iterate through the list of commit hashes, checking out each commit in turn using the checkoutCommit function, which uses the os/exec package to run the git checkout command.

- Search for valid AWS IAM keys in the code at each commit using the searchIAMKeysInRepo function, which searches through all the files in the repository for strings that match the pattern of an AWS Access Key ID and Secret Access Key.

- Verify the validity of the keys found using the validateIAMKeys function, which uses the AWS SDK for Go to make API calls to AWS to check whether the keys are valid.

Report the valid keys found by printing them to the console.

The aws-iam-keys-finder program is designed to be flexible and scalable, so it can be used to scan multiple repositories and can be easily extended to include additional validation checks.

Overall, the program provides a simple and effective solution for identifying AWS IAM keys in public GitHub repositories, which can help organizations to protect their sensitive data and ensure the security of their AWS resources.