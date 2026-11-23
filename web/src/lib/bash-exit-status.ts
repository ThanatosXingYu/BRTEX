// Norma appends this status after the collected process output. The optional
// untrusted-output boundary encloses the status as well as the process output.
export function reportsBashExit127(output: string): boolean {
  let body = output.trimEnd();
  const frame = /^<<<UNTRUSTED_OUTPUT ([^\r\n]+)>>>>\r?\n/.exec(body);
  if (frame) {
    const end = `<<<END_UNTRUSTED_OUTPUT ${frame[1]}>>>>`;
    if (!body.endsWith(`\n${end}`)) return false;
    body = body.slice(0, -(end.length + 1)).trimEnd();
  }
  return /\r?\n\r?\n\[exit code 127\]$/.test(body);
}
