# Contributing to Foundries Update Server

Hi there!
We’re thrilled that you’d like to contribute to this project.
Your help is essential for keeping this project great and for making it better.

## Branching Strategy

In general, contributors should develop on branches based off of `main` and pull requests should be made against `main`.

## Submitting a pull request

1. Please read our [code of conduct](CODE-OF-CONDUCT.md) and [license](LICENSE.txt).
1. [Fork](https://github.com/foundriesio/update-server/fork) and clone the repository.

    ```bash
    git clone https://github.com/<username>/fioup.git
    ```

1. Create a new branch based on `main`:

    ```bash
    git checkout -b <my-branch-name> main
    ```

1. Create an upstream `remote` to make it easier to keep your branches up-to-date:

    ```bash
    git remote add upstream https://github.com/foundriesio/update-server.git
    ```

1. Make your changes, add tests, and make sure the tests still pass.
1. Commit your changes using the [DCO](https://developercertificate.org/). You can attest to the DCO by commiting with the **-s** or **--signoff** options or manually adding the "Signed-off-by":

    ```bash
    git commit -s -m "Really useful commit message"`
    ```

1. After committing your changes on the topic branch, sync it with the upstream branch:

    ```bash
    git pull --rebase upstream main
    ```

1. Push to your fork.

    ```bash
    git push -u origin <my-branch-name>
    ```

    The `-u` is shorthand for `--set-upstream`. This will set up the tracking reference so subsequent runs of `git push` or `git pull` can omit the remote and branch.

1. [Submit a pull request](https://github.com/foundriesio/fioup/pulls) from your branch to `main`.
1. Pat yourself on the back and wait for your pull request to be reviewed.

Here are a few things you can do that will increase the likelihood of your pull request to be accepted:

- Follow the existing style where possible.
- Write tests.
- Keep your change as focused as possible.
  If you want to make multiple independent changes, please consider submitting them as separate pull requests.
- Write a [good commit message](https://tbaggery.com/2008/04/19/a-note-about-git-commit-messages.html).
- It's a good idea to arrange a discussion with other developers to ensure there is consensus on large features, architecture changes, and other core code changes. PR reviews will go much faster when there are no surprises.

## Translations

UI strings are translated with GNU gettext 1.0 or later: `brew install gettext`
on macOS, `sudo apt-get install gettext` on Debian and Ubuntu. `docs/i18n.md`
has the full flow.

To add or change a message:

1. Wrap the text in a template with `{{.L.T "message"}}`, or for plurals
   `{{.L.N "singular" "plural" .Count}}`.
1. Run `make i18n-extract` to regenerate `server/ui/i18n/po/ui.pot`.
1. Run `make i18n-merge` to update each `server/ui/i18n/po/<lang>.po`, then
   translate the new entries.
1. Run `make i18n-compile`, then commit the `.pot`, `.po` and `.mo` files.

Only direct `.T(` and `.N(` calls are extracted: method values and template
aliases such as `$l := .L` are not.

`make i18n-check` runs extract, merge and compile, fails if the committed files
differ or any translation is untranslated or fuzzy, then runs the i18n Go
tests. Commit before running it.
