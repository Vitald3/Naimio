const extensionFor = (name: string, mimeType?: string) => {
  const fromName = name.split(".").pop()?.trim().toUpperCase();
  if (fromName && fromName !== name.toUpperCase() && fromName.length <= 6)
    return fromName;
  if (mimeType?.startsWith("image/"))
    return mimeType.slice(6).toUpperCase().replace("JPEG", "JPG");
  if (mimeType === "application/pdf") return "PDF";
  return "FILE";
};

export default function FileTypeBadge({
  name,
  mimeType,
}: {
  name: string;
  mimeType?: string;
}) {
  const extension = extensionFor(name, mimeType);
  return (
    <span
      className={`file-type-badge file-type-badge--${extension.toLowerCase()}`}
      aria-label={`Файл ${extension}`}
    >
      <svg viewBox="0 0 28 34" aria-hidden="true">
        <path d="M4 1h13l7 7v25H4z" />
        <path d="M17 1v8h7" />
      </svg>
      <b>{extension}</b>
    </span>
  );
}
