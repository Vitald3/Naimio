import test from "node:test";
import assert from "node:assert/strict";

const translitMap = {
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

function slugify(text) {
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

test("slugify transliterates Cyrillic correctly", () => {
  assert.equal(
    slugify("Как найти надежного исполнителя"),
    "kak-nayti-nadezhnogo-ispolnitelya",
  );
  assert.equal(
    slugify("Мобильные приложения & боты"),
    "mobilnye-prilozheniya-boty",
  );
  assert.equal(
    slugify("Разработка на Go, Node.js и PostgreSQL!"),
    "razrabotka-na-go-node-js-i-postgresql",
  );
  assert.equal(
    slugify("  --- Много  пробелов   и   знаков ??? --- "),
    "mnogo-probelov-i-znakov",
  );
});

test("slugify handles empty and single char inputs", () => {
  assert.equal(slugify(""), "");
  assert.equal(slugify("   "), "");
  assert.equal(slugify("Я"), "ya");
  assert.equal(slugify("---"), "");
});
