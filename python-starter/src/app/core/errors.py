from __future__ import annotations


class AppError(Exception):
    pass


class ConfigurationError(AppError):
    pass


class ServiceError(AppError):
    pass


class NotFoundError(ServiceError):
    pass


class ValidationError(AppError):
    pass
