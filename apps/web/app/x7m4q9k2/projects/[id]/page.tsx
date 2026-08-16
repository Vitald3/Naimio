"use client";
import { useParams } from "next/navigation";
import AdminContentDetail from "../../content-detail";
export default function Page(){const params=useParams<{id:string}>();return <AdminContentDetail endpoint="projects" kind="PROJECT" id={params.id}/>}
