import type { MetadataRoute } from "next";
import { canonical } from "./seo";
type Item={path:string;updated_at:string};type BlogPost={slug:string;updated_at:string};
export const revalidate=0;
export default async function sitemap():Promise<MetadataRoute.Sitemap>{
  const fixed=["/","/categories","/freelancers","/services","/projects","/vacancies","/education","/check-offer"].map(url=>({url:canonical(url),changeFrequency:"daily" as const,priority:url==="/"?1:.7}));
  const base=process.env.API_BASE_URL??"http://localhost:8080";
  try{
    const [seoResponse,blogResponse]=await Promise.all([fetch(`${base}/api/v1/seo/sitemap`,{next:{revalidate:3600}}),fetch(`${base}/api/v1/blog?page_size=30`,{cache:"no-store"})]);
    const dynamic=seoResponse.ok?(((await seoResponse.json()).data??[]) as Item[]).filter((v:Item)=>v.path.startsWith("/")&&!v.path.includes("?")).map((v:Item)=>({url:canonical(v.path),lastModified:new Date(v.updated_at),changeFrequency:"weekly" as const,priority:v.path.startsWith("/price/")?0.8:0.6})):[];
    const blogBody=blogResponse.ok?await blogResponse.json():null;const posts:BlogPost[]=blogBody?.data?.items??[];const blog=blogResponse.ok?[{url:canonical("/blog"),changeFrequency:"daily" as const,priority:.75},...posts.map(v=>({url:canonical(`/blog/${v.slug}`),lastModified:new Date(v.updated_at),changeFrequency:"monthly" as const,priority:.7}))]:[];
    return [...fixed,...dynamic,...blog];
  }catch{return fixed}
}
