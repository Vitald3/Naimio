import { canonical, jsonLD } from "./seo";

export type BreadcrumbItem = { label: string; href?: string };

export default function Breadcrumbs({ items }: { items: BreadcrumbItem[] }) {
  const normalized = items.filter((item) => item.label.trim());
  const schema = {
    "@context": "https://schema.org",
    "@type": "BreadcrumbList",
    itemListElement: normalized.map((item, index) => ({
      "@type": "ListItem",
      position: index + 1,
      name: item.label,
      ...(item.href ? { item: canonical(item.href) } : {}),
    })),
  };
  return (
    <>
      <nav className="breadcrumbs" aria-label="Хлебные крошки">
        <ol>
          {normalized.map((item, index) => (
            <li key={`${item.label}-${index}`}>
              {item.href && index < normalized.length - 1 ? <a href={item.href}>{item.label}</a> : <span aria-current={index === normalized.length - 1 ? "page" : undefined}>{item.label}</span>}
            </li>
          ))}
        </ol>
      </nav>
      <script type="application/ld+json" dangerouslySetInnerHTML={{ __html: jsonLD(schema) }} />
    </>
  );
}
