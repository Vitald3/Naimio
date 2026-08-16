import type { ReactNode } from "react";
import sanitizeHtml from "sanitize-html";

function inline(value: string): ReactNode[] {
  const out: ReactNode[] = [];
  const pattern = /(\*\*[^*]+\*\*|\[[^\]]+\]\(https?:\/\/[^\s)]+\))/g;
  let cursor = 0;
  let match: RegExpExecArray | null;
  while ((match = pattern.exec(value)) !== null) {
    if (match.index > cursor) out.push(value.slice(cursor, match.index));
    const token = match[0];
    if (token.startsWith("**")) out.push(<strong key={`${match.index}-strong`}>{token.slice(2, -2)}</strong>);
    else {
      const link = token.match(/^\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)$/);
      if (link) out.push(<a key={`${match.index}-link`} href={link[2]} target="_blank" rel="noreferrer nofollow">{link[1]}</a>);
      else out.push(token);
    }
    cursor = match.index + token.length;
  }
  if (cursor < value.length) out.push(value.slice(cursor));
  return out;
}

export default function FormattedText({ value }: { value: string }) {
  if (/^\s*</.test(value)) {
    const safe=sanitizeHtml(value,{allowedTags:["p","h2","h3","strong","em","ul","ol","li","a","br"],allowedAttributes:{a:["href","target","rel"]},allowedSchemes:["http","https"],transformTags:{a:(tagName,attrs)=>({tagName,attribs:{...attrs,target:"_blank",rel:"noopener noreferrer nofollow"}})}});
    return <div className="formatted-text" dangerouslySetInnerHTML={{__html:safe}}/>;
  }
  const lines = value.replace(/\r\n/g, "\n").split("\n");
  const nodes: ReactNode[] = [];
  let list: { ordered: boolean; items: string[] } | null = null;
  const flush = () => {
    if (!list) return;
    const Tag = list.ordered ? "ol" : "ul";
    nodes.push(<Tag key={`list-${nodes.length}`}>{list.items.map((item, index) => <li key={index}>{inline(item)}</li>)}</Tag>);
    list = null;
  };
  lines.forEach((raw, index) => {
    const line = raw.trimEnd();
    if (!line.trim()) { flush(); return; }
    const heading = line.match(/^(#{1,3})\s+(.+)$/);
    if (heading) {
      flush();
      const level = Math.min(4, heading[1].length + 1);
      const Tag = `h${level}` as "h2" | "h3" | "h4";
      nodes.push(<Tag key={`h-${index}`}>{inline(heading[2])}</Tag>);
      return;
    }
    const unordered = line.match(/^[-•]\s+(?:\[.\]\s+)?(.+)$/);
    const ordered = line.match(/^\d+[.)]\s+(.+)$/);
    if (unordered || ordered) {
      const isOrdered = !!ordered;
      const item = (ordered || unordered)![1];
      if (!list || list.ordered !== isOrdered) { flush(); list = { ordered: isOrdered, items: [] }; }
      list.items.push(item);
      return;
    }
    flush();
    nodes.push(<p key={`p-${index}`}>{inline(line)}</p>);
  });
  flush();
  return <div className="formatted-text">{nodes}</div>;
}
