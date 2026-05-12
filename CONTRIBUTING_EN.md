# Contributing to Higress

Your interest in contributing to Higress is warmly welcomed. First, we encourage this kind of willing very much. And here is a list of contributing guide for you.

[[中文贡献文档](./CONTRIBUTING_CN.md)]

## Topics

- [Contributing to Higress](#contributing-to-higress)
  - [Topics](#topics)
  - [Reporting security issues](#reporting-security-issues)
  - [Reporting general issues](#reporting-general-issues)
  - [Code and doc contribution](#code-and-doc-contribution)
    - [Workspace Preparation](#workspace-preparation)
    - [Branch Definition](#branch-definition)
    - [Commit Rules](#commit-rules)
      - [Commit Message](#commit-message)
      - [Commit Content](#commit-content)
    - [PR Description](#pr-description)
  - [Test case contribution](#test-case-contribution)
  - [Engage to help anything](#engage-to-help-anything)
  - [Code Style](#code-style)

## Reporting security issues

Security issues are always treated seriously. As our usual principle, we discourage anyone to spread security issues. If you find a security issue of Higress, please do not discuss it in public and even do not open a public issue. Instead please follow the process described in [`SECURITY.md`](./SECURITY.md) to report vulnerabilities privately.

## Reporting general issues

To be honest, we regard every user of Higress as a very kind contributor. After experiencing Higress, you may have 
some feedback for the project. Then feel free to open an issue via [NEW ISSUE](https://github.com/alibaba/higress/issues/new/choose).

Since we collaborate project Higress in a distributed way, we appreciate **WELL-WRITTEN**, **DETAILED**, **EXPLICIT** issue reports. To make the communication more efficient, we wish everyone could search if your issue is an existing one in the searching list. If you find it existing, please add your details in comments under the existing issue instead of opening a brand new one.

To make the issue details as standard as possible, we setup an [ISSUE TEMPLATE](./.github/ISSUE_TEMPLATE) for issue reporters. Please **BE SURE** to follow the instructions to fill fields in template.

There are a lot of cases when you could open an issue:

* bug report
* feature request
* performance issues
* feature proposal
* feature design
* help wanted
* doc incomplete
* test improvement
* any questions on project
* and so on

Also we must remind that when filling a new issue, please remember to remove the sensitive data from your post. Sensitive data could be password, secret key, network locations, private business data and so on.

## Code and doc contribution

Every action to make project Higress better is encouraged. On GitHub, every improvement for Higress could be via a PR (short for pull request).

* If you find a typo, try to fix it!
* If you find a bug, try to fix it!
* If you find some redundant codes, try to remove them!
* If you find some test cases missing, try to add them!
* If you could enhance a feature, please **DO NOT** hesitate!
* If you find code implicit, try to add comments to make it clear!
* If you find code ugly, try to refactor that!
* If you can help to improve documents, it could not be better!
* If you find document incorrect, just do it and fix that!
* ...

Actually it is impossible to list them completely. Just remember one principle:

> WE ARE LOOKING FORWARD TO ANY PR FROM YOU.

Since you are ready to improve Higress with a PR, we suggest you could take a look at the PR rules here.

* [Workspace Preparation](#workspace-preparation)
* [Branch Definition](#branch-definition)
* [Commit Rules](#commit-rules)
* [PR Description](#pr-description)

### Workspace Preparation

To put forward a PR, we assume you have registered a GitHub ID. Then you could finish the preparation in the following steps:

1. **FORK** Higress to your repository. To make this work, you just need to click the button Fork in right-left of[alibaba/higress](https://github.com/alibaba/higress) main page. Then you will end up with your repository in 
   `https://github.com/<your-username>/higress`, in which `your-username` is your GitHub username.

1. **CLONE** your own repository to develop locally. Use `git clone git@github.com:<your-username>/higress.git` to clone repository to your local machine. Then you can create new branches to finish the change you wish to make.

1. **Set Remote** upstream to be `git@github.com:alibaba/higress.git` using the following two commands:

```bash
git remote add upstream git@github.com:alibaba/higress.git
git remote set-url --push upstream no-pushing
```

With this remote setting, you can check your git remote configuration like this:

```shell
$ git remote -v
origin     git@github.com:<your-username>/higress.git (fetch)
origin     git@github.com:<your-username>/higress.git (push)
upstream   git@github.com:alibaba/higress.git (fetch)
upstream   no-pushing (push)
```

Adding this, we can easily synchronize local branches with upstream branches.

### Branch Definition

Right now we assume every contribution via pull request is for [branch main](https://github.com/alibaba/higress/tree/main) in Higress. Before contributing, be aware of branch definition would help a lot.

As a contributor, keep in mind again that every contribution via pull request is for branch main. While in project Higress, there are several other branches, we generally call them release branches (such as 0.6.0,0.6.1), feature branches, hotfix branches.

When officially releasing a version, there will be a release branch and named with the version number.

After the release, we will merge the commit of the release branch into the main branch.

When we find that there is a bug in a certain version, we will decide to fix it in a later version or fix it in a specific hotfix version. When we decide to fix the hotfix version, we will checkout the hotfix branch based on the corresponding release branch, perform code repair and verification, and merge it into the main branch.

For larger features, we will pull out the feature branch for development and verification.


### Commit Rules

Actually in Higress, we take two rules serious when committing:

* [Commit Message](#commit-message)
* [Commit Content](#commit-content)

#### Commit Message

Commit message could help reviewers better understand what is the purpose of submitted PR. It could help accelerate the code review procedure as well. We encourage contributors to use **EXPLICIT** commit message rather than ambiguous message. In general, we advocate the following commit message type:

* docs: xxxx. For example, "docs: add docs about Higress cluster installation".
* feature: xxxx.For example, "feature: use higress config instead of istio config".
* bugfix: xxxx. For example, "bugfix: fix panic when input nil parameter".
* refactor: xxxx. For example, "refactor: simplify to make codes more readable".
* test: xxx. For example, "test: add unit test case for func InsertIntoArray".
* other readable and explicit expression ways.

On the other side, we discourage contributors from committing message like the following ways:

* ~~fix bug~~
* ~~update~~
* ~~add doc~~

If you get lost, please see [How to Write a Git Commit Message](http://chris.beams.io/posts/git-commit/) for a start.

#### Commit Content

Commit content represents all content changes included in one commit. We had better include things in one single commit which could support reviewer's complete review without any other commits' help. In another word, contents in one single commit can pass the CI to avoid code mess. In brief, there are three minor rules for us to keep in mind:

* avoid very large change in a commit;
* complete and reviewable for each commit.
* check git config(`user.name`, `user.email`) when committing to ensure that it is associated with your GitHub ID.

```bash
git config --get user.name
git config --get user.email
```

* when submitting pr, please add a brief description of the current changes to the X.X.X.md file under the 'changes/' folder


In addition, in the code change part, we suggest that all contributors should read the [code style of Higress](#code-style).

No matter commit message, or commit content, we do take more emphasis on code review.


### PR Description

PR is the only way to make change to Higress project files. To help reviewers better get your purpose, PR description could not be too detailed. We encourage contributors to follow the [PR template](./.github/PULL_REQUEST_TEMPLATE.md) to finish the pull request.

#### Special Requirements for AI Coding Tool Usage

If you use AI Coding tools (such as Cursor, GitHub Copilot, etc.) to generate PRs, we have the following **strict requirements**:

**For new standalone plugin scenarios** (e.g., newly implemented wasm plugins or golang-filter plugins):
- You **MUST** create a `design/` directory under the plugin directory
- Place the design document you provided to the AI Coding tool in the `design/` directory
- Provide an AI Coding summary in the PR description

**For regular updates/changes scenarios**:
- Provide the prompts/instructions you gave to the AI Coding tool in the PR description
- Provide an AI Coding summary in the PR description

**AI Coding Summary should include**:
- Key decisions made
- Major changes implemented
- Important considerations or limitations

**Review Priority Notice**:
- If you use AI Coding tools but do not follow the above requirements, your PR review priority will be **lowered**
- We **cannot guarantee** timely reviews for AI Coding PRs that do not meet these requirements
- If the PR is not completed using AI Coding tools, these additional requirements do not apply

The purpose of these requirements is to ensure that AI-generated code is adequately documented and traceable, facilitating code review and subsequent maintenance. By requiring prompts/design documents, we can better understand the development intent and context.

### Pre-development preparation

```shell
make prebuild && go mod tidy
```

## Test case contribution

Any test case would be welcomed. Currently, Higress function test cases are high priority.

### Test requirements for new functionality

- **New Wasm plugins**: MUST include unit tests with at least 30% code coverage (enforced by CI).
- **New core features**: SHOULD include unit tests and, where applicable, E2E conformance test cases.
- **Bug fixes**: SHOULD include a regression test that reproduces the bug.
- **Patch coverage**: New or changed code must meet a 50% coverage target for the patch (enforced by Codecov via `codecov.yml`).

### How to write tests

* For unit tests, create a test file named `xxxTest.go` in the test directory of the same module.
* For integration tests, you can put the integration test in the test directory.
* For Wasm plugin E2E tests, add test cases in `test/e2e/conformance/tests/`. See [test/README.md](./test/README.md) for details.
## Engage to help anything

We choose GitHub as the primary place for Higress to collaborate. So the latest updates of Higress are always here. Although contributions via PR is an explicit way to help, we still call for any other ways.

* reply to other's issues if you could;
* help solve other user's problems;
* help review other's PR design;
* help review other's codes in PR;
* discuss about Higress to make things clearer;
* advocate Higress technology beyond GitHub;
* write blogs on Higress and so on.


## Code Style
//TBD
In a word, **ANY HELP IS CONTRIBUTION.**
