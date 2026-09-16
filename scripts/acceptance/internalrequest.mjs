// Where an internal request is sent from and how its answer is read. The internal listeners publish
// no port, so a check reaches them the way the services do: from a container of the Compose network,
// which is also how a caller inside the deployment would reach them. The frontend container is the
// one this stack already runs, so its curl is the client and nothing extra is started.
import { execFileSync } from 'node:child_process';
import { repositoryRoot } from '../service.mjs';

const CLIENT_SERVICE = 'frontend';
const CLIENT_TIMEOUT_MS = 60_000;

/** The listener of the API that carries the internal operations and publishes no port. */
export const API_URL = 'http://api:8080';

/** The listener of the mail stub that carries its internal operations and publishes no port. */
export const MAILSTUB_URL = 'http://mailstub:8080';

/**
 * The origin the local profile allows, which a mutation sent straight to a listener has to carry:
 * the check follows the contract, and reaching the API without the proxy does not make a mutation
 * any less of one.
 */
export const ALLOWED_ORIGIN = 'http://127.0.0.1:8080';

/**
 * One request to a listener no port publishes, with everything a refusal or an acceptance is read
 * from: the status, the body, and the parsed body when it is JSON. curl prints the status after the
 * body, because the body alone cannot say whether a refusal was a 401 or a 404.
 *
 * A body is handed to curl on its standard input rather than as an argument: a request at the limit
 * the contract declares is far longer than a command line can carry on the host this runs on, and an
 * argument that does not fit would fail the check for a reason that has nothing to do with the limit.
 */
export function internalRequest(baseUrl, path, { method = 'GET', body, headers = {}, origin } = {}) {
  const args = ['-s', '-o', '-', '-w', '\n%{http_code}', '-X', method];
  if (body !== undefined) {
    args.push('-H', 'Content-Type: application/json', '--data-binary', '@-');
  }
  if (origin !== undefined) args.push('-H', `Origin: ${origin}`);
  for (const [name, value] of Object.entries(headers)) args.push('-H', `${name}: ${value}`);

  const output = runClient([...args, `${baseUrl}${path}`], body ?? '');
  const separator = output.lastIndexOf('\n');
  const status = Number(output.slice(separator + 1));
  const payload = output.slice(0, separator);
  return { status: Number.isInteger(status) ? status : 0, body: payload, json: parseJson(payload) };
}

/** One command sent with a bearer credential, which is what an internal capability is presented as. */
export function bearer(token) {
  return { Authorization: `Bearer ${token}` };
}

/** Runs curl inside the client container and returns everything it printed. */
function runClient(args, input) {
  try {
    return execFileSync(
      'docker',
      ['compose', 'run', '--rm', '-T', '--no-deps', '--entrypoint', 'curl', CLIENT_SERVICE, ...args],
      {
        cwd: repositoryRoot,
        encoding: 'utf8',
        input,
        stdio: ['pipe', 'pipe', 'pipe'],
        timeout: CLIENT_TIMEOUT_MS,
      },
    );
  } catch (failure) {
    return `${failure.stdout ?? ''}${failure.stderr ?? ''}`;
  }
}

function parseJson(text) {
  try {
    return JSON.parse(text);
  } catch {
    return null;
  }
}
