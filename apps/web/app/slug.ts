import { useEffect, useRef, useState } from "react";

const translitMap: Record<string, string> = {
  а: "a",
  б: "b",
  в: "v",
  г: "g",
  д: "d",
  е: "e",
  ё: "yo",
  ж: "zh",
  з: "z",
  и: "i",
  й: "y",
  к: "k",
  л: "l",
  м: "m",
  н: "n",
  о: "o",
  п: "p",
  р: "r",
  с: "s",
  т: "t",
  у: "u",
  ф: "f",
  х: "h",
  ц: "ts",
  ч: "ch",
  ш: "sh",
  щ: "sch",
  ъ: "",
  ы: "y",
  ь: "",
  э: "e",
  ю: "yu",
  я: "ya",
};

/**
 * Transliterates Russian / Cyrillic text and converts it into a clean, URL-safe slug.
 * Example: "Как найти фрилансера для проекта" -> "kak-naiti-frilansera-dlya-proekta"
 */
export function slugify(text: string): string {
  if (!text) return "";
  const lower = text.toLowerCase().trim();
  const transliterated = lower
    .split("")
    .map((char) => translitMap[char] ?? char)
    .join("");

  return transliterated
    .normalize("NFKD")
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .replace(/-{2,}/g, "-");
}

/**
 * React hook for managing linked Title -> Slug fields:
 * - Automatically generates slug while user types title.
 * - Stops overwriting if user manually edits the slug input.
 */
export function useAutoSlug({
  initialSlug = "",
  title,
  onSlugChange,
}: {
  initialSlug?: string;
  title: string;
  onSlugChange: (slug: string) => void;
}) {
  const [isManual, setIsManual] = useState(Boolean(initialSlug));
  const prevTitle = useRef(title);

  useEffect(() => {
    if (!isManual && title !== prevTitle.current) {
      const generated = slugify(title);
      onSlugChange(generated);
    }
    prevTitle.current = title;
  }, [title, isManual, onSlugChange]);

  const handleSlugInput = (value: string) => {
    setIsManual(true);
    const cleaned = value.toLowerCase().replace(/[^a-z0-9-]/g, "");
    onSlugChange(cleaned);
  };

  const resetManual = () => {
    setIsManual(false);
    onSlugChange(slugify(title));
  };

  return { isManual, handleSlugInput, resetManual };
}
