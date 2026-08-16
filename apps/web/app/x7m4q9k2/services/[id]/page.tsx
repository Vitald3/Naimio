"use client";
import { useParams } from "next/navigation";
import AdminContentDetail from "../../content-detail";
export default function Page(){const params=useParams<{id:string}>();return <AdminContentDetail endpoint="services" kind="SERVICE" id={params.id}/>}
