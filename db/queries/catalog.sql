-- name: ListActiveCategories :many
SELECT id,parent_id,slug,name,description,sort_order,is_active FROM categories
WHERE is_active=true ORDER BY sort_order,name LIMIT 1000;

-- name: SearchActiveSkills :many
SELECT id,slug,name,is_active FROM skills WHERE is_active=true
AND ($1::text='' OR name ILIKE '%'||$1||'%' OR slug ILIKE '%'||$1||'%') ORDER BY name LIMIT 50;

-- Admin mutations are role-checked by the repository and bounded by the category depth trigger.
