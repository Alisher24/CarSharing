/**
 * The frames of the change stream, read from the bytes of one HTTP response. The stream declares no
 * identifier and carries no replay, so nothing here remembers a frame it has already read: a frame
 * is handed on once, and a connection that ends is repaired by reading the REST snapshots again.
 */

/** One complete frame: the field that named it and the data it carried, when it carried any. */
export type Frame = { event: string; data?: string };

// A frame delimiter is a blank line. The server writes "\n\n", and a \r\n source is accepted too.
const FRAME_SEPARATOR = /\r?\n\r?\n/;

/**
 * frameLines turns one frame into its fields, or into undefined when it names no event. A comment,
 * an empty line and a field this contract does not declare are dropped rather than guessed at.
 */
export function frameLines(frame: string): Frame | undefined {
  let event = '';
  const data: string[] = [];

  for (const line of frame.split(/\r?\n/)) {
    if (line.startsWith(':')) continue;
    if (line.startsWith('event:')) event = fieldValue(line);
    if (line.startsWith('data:')) data.push(fieldValue(line));
  }

  if (event === '') return undefined;
  return data.length === 0 ? { event } : { event, data: data.join('\n') };
}

/**
 * readFrames yields one Frame per complete frame of a response body. A chunk that ends in the
 * middle of a frame is held until the rest of it arrives, which is why the reading is a function
 * over the whole body rather than over one chunk.
 */
export async function* readFrames(chunks: AsyncIterable<Uint8Array>): AsyncGenerator<Frame> {
  const decoder = new TextDecoder();
  let held = '';

  for await (const chunk of chunks) {
    held += decoder.decode(chunk, { stream: true });

    const frames = held.split(FRAME_SEPARATOR);
    held = frames.pop() ?? '';

    for (const frame of frames) {
      const parsed = frameLines(frame);
      if (parsed !== undefined) yield parsed;
    }
  }
}

function fieldValue(line: string): string {
  return line.slice(line.indexOf(':') + 1).replace(/^ /, '');
}
