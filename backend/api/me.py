from fastapi import APIRouter, Depends, HTTPException, Query
from starlette.concurrency import run_in_threadpool

from backend.dependencies import current_identity
from bot.application import AdminQueryService
from bot.application.auth_service import WebIdentity


router = APIRouter(prefix="/me", tags=["current-user"])
queries = AdminQueryService()


@router.get("")
async def my_profile(identity: WebIdentity = Depends(current_identity)):
    user = await run_in_threadpool(queries.get_user, identity.tg)
    if not user:
        raise HTTPException(status_code=404, detail="用户不存在")
    return {
        **user,
        "roles": identity.roles,
        "permissions": sorted(identity.permissions),
        "auth_method": identity.auth_method,
    }


@router.get("/point-transactions")
async def my_point_transactions(
    limit: int = Query(20, ge=1, le=100),
    offset: int = Query(0, ge=0),
    identity: WebIdentity = Depends(current_identity),
):
    return {
        "items": await run_in_threadpool(
            queries.point_transactions,
            identity.tg,
            limit,
            offset,
        ),
        "limit": limit,
        "offset": offset,
    }
