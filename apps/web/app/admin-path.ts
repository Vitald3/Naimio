export const STAFF_BASE_PATH = "/x7m4q9k2";
export const STAFF_LOGIN_PATH = `${STAFF_BASE_PATH}/login`;
export const STAFF_ROLES = new Set(["MODERATOR", "ADMIN", "SUPER_ADMIN"]);
export function isStaffRoles(roles?: string[] | null) {
  return !!roles?.some((role) => STAFF_ROLES.has(role));
}
