// The change signals the acceptance suites read. A suite opens a stream, waits for the frame it
// asserts about, and observes the connection ending, so a slow stack makes a check later rather than
// wrong.
import { serviceOrigin } from './client.mjs';

const FRAME_SEPARATOR = '\n\n';
const FIELD_SEPARATOR = '\n';

/** How long a check waits for a frame before it reports the stream as silent. */
export const STREAM_PATIENCE_MS = 20_000;

/**
 * Opens one SSE connection and reads it in the background. The status and the headers are returned as
 * they arrived, so a suite asserts a refused connection as well as a served one; a connection the
 * service refuses answers with the JSON error contract and no frames at all.
 */
export async function watchEventStream(path, { cookie } = {}) {
  const response = await fetch(serviceOrigin + path, {
    headers: { Accept: 'text/event-stream', ...(cookie ? { Cookie: cookie } : {}) },
  });
  const stream = {
    status: response.status,
    headers: response.headers,
    frames: [],
    ended: false,
    body: '',
    stop: null,
  };
  if (!response.ok) {
    stream.body = await response.text();
    stream.ended = true;
    return stream;
  }

  const closing = new Promise((resolve) => {
    stream.stop = resolve;
  });
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let pending = '';
  void (async () => {
    try {
      for (;;) {
        const read = await Promise.race([reader.read(), closing.then(() => ({ stopped: true }))]);
        if (read.stopped || read.done) break;
        pending += decoder.decode(read.value, { stream: true });
        let separator = pending.indexOf(FRAME_SEPARATOR);
        while (separator >= 0) {
          stream.frames.push(parseFrame(pending.slice(0, separator)));
          pending = pending.slice(separator + FRAME_SEPARATOR.length);
          separator = pending.indexOf(FRAME_SEPARATOR);
        }
      }
    } catch {
      // A connection the server closed while it was being read is an end of the stream, which is what
      // the checks below observe; nothing else can be read from it.
    } finally {
      stream.ended = true;
      await reader.cancel().catch(() => {});
    }
  })();
  return stream;
}

/** Waits for the first frame a predicate accepts, and reports what the stream carried instead. */
export async function waitForFrame(stream, accepts, patienceMs = STREAM_PATIENCE_MS) {
  return settle(stream, () => stream.frames.find(accepts), patienceMs, 'no matching frame');
}

/** Waits for the connection to end, which is how a suite observes a closed stream. */
export async function waitForEnd(stream, patienceMs = STREAM_PATIENCE_MS) {
  await settle(stream, () => stream.ended, patienceMs, 'the stream stayed open');
  stream.stop?.();
}

async function settle(stream, reached, patienceMs, complaint) {
  const deadline = Date.now() + patienceMs;
  for (;;) {
    const found = reached();
    if (found) return found;
    if (Date.now() > deadline) {
      throw new Error(`${complaint} within ${patienceMs} ms; saw ${describe(stream)}`);
    }
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
}

export function closeStream(stream) {
  stream.stop?.();
}

/** The change a frame announces, or null for a frame that announces no change. */
export function changeOf(frame) {
  if (!frame.data) return null;
  try {
    const parsed = JSON.parse(frame.data);
    return parsed.id && parsed.version ? parsed : null;
  } catch {
    return null;
  }
}

export function frameText(frame) {
  if (frame.comment) return frame.comment;
  return `${frame.event} ${frame.data}`;
}

function describe(stream) {
  return `${stream.frames.length} frames: ${stream.frames.map(frameText).join(' | ')}`;
}

/**
 * One frame of the stream: the event it names, its data, a retry hint, or a comment. The identifier
 * field is parsed too, so a check can prove the stream never sends one.
 */
function parseFrame(frame) {
  const parsed = { event: null, data: null, retry: null, id: null, comment: null };
  for (const line of frame.split(FIELD_SEPARATOR)) {
    if (line.startsWith(':')) parsed.comment = line.slice(1).trim();
    else if (line.startsWith('event:')) parsed.event = line.slice('event:'.length).trim();
    else if (line.startsWith('data:')) parsed.data = line.slice('data:'.length).trim();
    else if (line.startsWith('retry:')) parsed.retry = Number(line.slice('retry:'.length).trim());
    else if (line.startsWith('id:')) parsed.id = line.slice('id:'.length).trim();
  }
  return parsed;
}
