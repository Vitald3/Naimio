import type { MetadataRoute } from "next";
import { canonical } from "./seo";
export default function robots():MetadataRoute.Robots{return{rules:{userAgent:"*",allow:"/",disallow:["/api/","/dashboard/","/messages","/settings/","/admin/","/notifications","/favorites","/invite/","/create-project"]},sitemap:canonical("/sitemap.xml"),host:canonical("/")}}
