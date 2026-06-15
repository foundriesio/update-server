# Configuring User Authentication

The server supports different user authentication options.
Choose and configure the option that best fits your needs:

* **Google Single Sign On** — Configure server to authenticate accounts from
   a Google Workspace domain. This option is best for a server with a connection
   to the internet (Google), where your team uses Google Workspace identities.

* **GitHub Sign On** — Configure server to authenticate accounts from
   one or more GitHub organizations. This option is best for a
   server with a connection to the internet (GitHub), where your team
   uses one or more GitHub organizations.

   > [!IMPORTANT]
   > In order to prove a user is part of an organization, access must
   > be granted to one of the server's configured GitHub organizations during
   > the SSO login procedure.

* **Local users** — If your server has no internet connection, or you do not
   use GitHub or Google, you can also configure the server with locally
   managed users. This mode assumes no internet access, so advanced features
   like password reset (email) and MFA (via SMS) are not available.

## Configuring Google SSO

Assume your update server is hosted at `dg.example.com`. First go
to the [GCP OAuth2 Clients](https://console.cloud.google.com/auth/clients)
page. Click on "Create client". You'll be prompted for
the "Application type"—select `Web application` from the drop-down menu.
Next, give it a name like "Foundries Update Server".

Set the "Authorized JavaScript Origins" to a single entry:
`https://dg.example.com`.

Set the "Authorized redirect URIs" to a single entry:
`https://dg.example.com/auth/callback`.

> [!IMPORTANT]
> The `auth/callback` part of the URI is critical and must be this value.

After clicking "Create", you'll be presented with a pop-up dialog that includes
your Client ID and Secret. Make note of both these values. They are required
for the next step.

Copy `/contrib/auth-config-google.json` to `<configdir>/auth/auth-config.json`
and set the values:

* `Config.ClientID`
* `Config.ClientSecret`
* `Config.AllowedDomains` — e.g. If your company emails are `@example.com`, enter `example.com` here.
* `Config.BaseUrl` — For our example, `https://dg.example.com`.

## Configuring GitHub SSO

Assume your update server is hosted at `dg.example.com`. First go
to the GitHub [Developer Settings](https://github.com/settings/apps) page.
From here, select the "OAuth Apps" option from the sidebar, and click the
"New OAuth App" button. The "Application name" should be something descriptive
for you like "Foundries Update Server". The URL does not matter, but could
be `https://dg.example.com` for this example. The "Authorization callback URL"
is critical and must be in the form of `https://dg.example.com/auth/callback`.

You can then click "Register application". This will take you to a page where you
can manage the new application. The "Client ID" will be displayed in plain
text. You will also need to generate a client secret by clicking "Generate a new
client secret". These two values are required for the next step.

Copy `/contrib/auth-config-github.json` to `<configdir>/auth/auth-config.json`
and set the values:

* `Config.ClientID`
* `Config.ClientSecret`
* `Config.AllowedOrgs` — A user must be a member of one of the values here to login to the server.
* `Config.BaseUrl` — For our example, `https://dg.example.com`.

## Configuring Locally Managed Users

If you can not use an SSO provider, you can configure the server with locally
managed users.

Copy `contrib/auth-config-local.json` to `<configdir>/auth/auth-config.json`
and set these optional values:

* `Config.MinPasswordLength` — Set to enforce a minimum password length. `8` would require passwords be at least 8 characters. The default is 0—not enforced.
* `Config.PasswordAgeDays` — Set to require users to change their password every `PasswordAgeDays`. `180` would require a user to change their password every 180 days. The default is 0—not enforced.
* `Config.PasswordHistory` — Set this to prevent users from repeating old passwords. `5` means they must use 5 different passwords before repeating. The default is 0—not enforced.
* `Config.PasswordComplexityRules` — Set these options to require more complex passwords. Disabled by default.
  * `RequireUppercase` — If true, the password must contain a character `A-Z`.
  * `RequireLowercase` — If true, the password must contain a character `a-z`.
  * `RequireDigit` — If true, the password must contain a character `0-9`.
  * `RequireSpecialChar` — If set, the password must contain one of the characters in the string. A value of `!@#` would make the user include one of those characters in their password.

You will need to define the initial user by running:

```
  ./fioserver user-add --username <initial user name> --password <password>
```

## Configuring Authentication Rate Limits

The server employs configurable rate limits for authentication-related
operations such as:

* Login
* Password change/reset
* Invalid API tokens

There are two layers to the logic. The first layer is plain rate limiting
by IP address. By default, the server will allow 2 requests/second per
IP before returning HTTP 429 "Too many requests" responses. The IP will
then be blocked from making authentication operations for 30 seconds.

The second layer is for blocking IPs that have made more than 5 bad
authentication-related operations in a minute. The IP will be blocked
from making any authentication operations for 5 minutes when the limit
has been reached.

These values can be configured via `Config.RateLimits`:

* `AttemptsPerSecond` — Set to globally rate-limit authentication operations (login, password change/reset) allowed per IP/second. The default is 2. Requests will then be blocked for `AttemptsBlockDurationSec` for the given IP.
* `AttemptsBlockDurationSec` — Set how long to block an IP that has been rate-limited by `AttemptsPerSecond`. The default will reject an IP for 30 seconds if it exceeds 2 authentication attempts per second.
* `BadAuthLimit` — Track how many bad password operations are made from a given account. The default is 5. If this value is exceeded, the given IP will be blocked for `BadAuthBlockDurationSec` from performing password related operations.
* `BadAuthBlockDurationSec` — Set how long to block an IP from performing authentication operations after exceeding `BadAuthLimit`. The default is 300 (5 minutes).
