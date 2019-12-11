# Contributing Guide

## Filing Bugs

Bug reports may be filed as an issue on GitHub, with the label `kind/bug`.
The label can be added by placing the [Prow](https://prow.svc.ci.openshift.org/command-help?repo=openshift%2Fsource-to-image)
command `/kind bug` in your issue.

A well-written bug report has the following format:

```markdown
**Is this a feature request or bug?**

/kind bug

**What went wrong?**

Enter a description of what went wrong.

**Steps to reproduce:**

1. Step 1
2. Step 2
3. Step 3

**Expected results:**

Enter what you expected to happen.

**Actual results:**

Enter what actually occurred, including command line output

**Version:**

s2i: Enter output of `s2i version`
docker: Enter the output of `docker version` if you are using docker to build container images.

**Additional info:**

Add any relevant context here.
```

## Submitting Feature Requests

Feature requests are likewise submitted as GitHub issues, with the `kind/feature` label.
The label can be added by placing the [Prow](https://prow.svc.ci.openshift.org/command-help?repo=openshift%2Fsource-to-image)
command `/kind feature` in your issue.

A well-written feature request has the following format:

```markdown
**Is this a feature request or bug?**

/kind feature

**User Stories**

As a developer using source-to-image
I would like ...
So that ...

(you may add more than one story to provide further use cases to consider)

**Additional info:**

Add any relevant context or deeper description here.
```

## Submitting a Pull Request

You can contribute to source-to-image by submitting a pull request ("PR").
Any member of the OpenShift organization can review your PR and signal their approval with the
`/lgtm` command. Only approvers listed in the [OWNERS](OWNERS) file may add the `approved` label to
your PR. In order for PR to merge, it must have the following:

1. The `approved` label
2. The `lgtm` label
3. All tests passing in CI, which is managed by Prow.

If you are not a member of the OpenShift GitHub organization, an OpenShift team member will need to
add the `ok-to-test` label to your PR for our CI tests to run.

### Feature or Bugfix PRs

Pull requests which implement a feature or fix a bug should consist of a single commit with the
full code changes. Commit messages should have the following structure:

```text
<Title - under 50 characters>

<Body - under 100 characters per line>

<Footer>
```

The title is required, and should be under 50 characters long. This is a short description of your
change.

The body is optional, and may contain a longer description of the changes in the commit. Lines
should not exceed 100 characters in length for readability.

The footer is optional, and may contain information such as sign-off lines. If your code is related
to a GitHub issue, add it here with the text `Fixes xxx`.

### Work In Progress PRs

Contributors who would like feedback but are still making code modifications can create "work in
progress" PRs by adding "WIP" to the PR title. Once work is done and ready for final review, please
remove "WIP" from your PR title and [squash your commits](https://medium.com/@slamflipstrom/a-beginners-guide-to-squashing-commits-with-git-rebase-8185cf6e62ec).

### Updating dependencies

Pull requests which update dependencies are an exception to the "one commit" rule.
To better track dependency changes, PRs with dependency updates should have the following structure:

1. A commit with changes to `go.mod` and `go.sum` declaring the new or updated dependencies.
2. A commit with updates to vendored code, with `bump(<modules>)` in the title:
   1. If only a small number of modules are updated, list them in the parentheses.
      Example: `bump(containers/image/v5):`
   2. If several modules are updated, use `bump(*)` as the title.
   3. Include a reason for updating dependencies in the commit body.
3. If necessary, add a commit with reactions to vendored code changes (ex - method signature
   updates).
4. If necessary, add a commit with code that takes advantage of the newly vendored code.

See [HACKING.md](docs/HACKING.md#dependency-management) for more information on how to update dependencies.
