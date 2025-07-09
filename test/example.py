"""
User management system in Python
"""

import asyncio
import logging
from abc import ABC, abstractmethod
from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Dict, List, Optional, Protocol, Union
from functools import wraps
import json

# Constants
MAX_USERS = 1000
DEFAULT_TIMEOUT = 5.0

# Global variables
user_count = 0
is_initialized = False

class UserStatus(Enum):
    """User status enumeration"""
    ACTIVE = "active"
    INACTIVE = "inactive"
    SUSPENDED = "suspended"
    DELETED = "deleted"

@dataclass
class User:
    """User entity class"""
    id: int
    name: str
    email: str
    status: UserStatus = UserStatus.ACTIVE
    created_at: datetime = field(default_factory=datetime.now)
    metadata: Dict[str, str] = field(default_factory=dict)
    
    def __post_init__(self):
        """Post-initialization processing"""
        if not self.email or '@' not in self.email:
            raise ValueError("Invalid email address")
    
    @property
    def display_name(self) -> str:
        """Get user display name"""
        return f"{self.name} ({self.email})"
    
    @property
    def is_active(self) -> bool:
        """Check if user is active"""
        return self.status == UserStatus.ACTIVE
    
    def activate(self) -> None:
        """Activate the user"""
        self.status = UserStatus.ACTIVE
    
    def deactivate(self) -> None:
        """Deactivate the user"""
        self.status = UserStatus.INACTIVE
    
    def suspend(self) -> None:
        """Suspend the user"""
        self.status = UserStatus.SUSPENDED
    
    def to_dict(self) -> Dict:
        """Convert user to dictionary"""
        return {
            'id': self.id,
            'name': self.name,
            'email': self.email,
            'status': self.status.value,
            'created_at': self.created_at.isoformat(),
            'metadata': self.metadata
        }
    
    @classmethod
    def from_dict(cls, data: Dict) -> 'User':
        """Create user from dictionary"""
        return cls(
            id=data['id'],
            name=data['name'],
            email=data['email'],
            status=UserStatus(data.get('status', UserStatus.ACTIVE.value)),
            created_at=datetime.fromisoformat(data.get('created_at', datetime.now().isoformat())),
            metadata=data.get('metadata', {})
        )

class UserRepository(Protocol):
    """User repository protocol"""
    
    async def get_by_id(self, user_id: int) -> Optional[User]:
        """Get user by ID"""
        ...
    
    async def save(self, user: User) -> User:
        """Save user"""
        ...
    
    async def delete(self, user_id: int) -> bool:
        """Delete user"""
        ...
    
    async def find_by_email(self, email: str) -> Optional[User]:
        """Find user by email"""
        ...
    
    async def list_all(self) -> List[User]:
        """List all users"""
        ...

class MemoryUserRepository:
    """In-memory user repository implementation"""
    
    def __init__(self):
        self._users: Dict[int, User] = {}
        self._next_id = 1
    
    async def get_by_id(self, user_id: int) -> Optional[User]:
        """Get user by ID"""
        return self._users.get(user_id)
    
    async def save(self, user: User) -> User:
        """Save user"""
        if user.id == 0:
            user.id = self._next_id
            self._next_id += 1
        self._users[user.id] = user
        return user
    
    async def delete(self, user_id: int) -> bool:
        """Delete user"""
        if user_id in self._users:
            del self._users[user_id]
            return True
        return False
    
    async def find_by_email(self, email: str) -> Optional[User]:
        """Find user by email"""
        for user in self._users.values():
            if user.email == email:
                return user
        return None
    
    async def list_all(self) -> List[User]:
        """List all users"""
        return list(self._users.values())

class UserNotFoundError(Exception):
    """User not found exception"""
    pass

class ValidationError(Exception):
    """Validation error exception"""
    pass

def validate_email(email: str) -> bool:
    """Validate email address"""
    import re
    pattern = r'^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$'
    return re.match(pattern, email) is not None

def log_method_call(func):
    """Decorator to log method calls"""
    @wraps(func)
    async def wrapper(*args, **kwargs):
        logger = logging.getLogger(__name__)
        logger.info(f"Calling {func.__name__} with args: {args[1:]} kwargs: {kwargs}")
        try:
            result = await func(*args, **kwargs)
            logger.info(f"{func.__name__} completed successfully")
            return result
        except Exception as e:
            logger.error(f"{func.__name__} failed with error: {e}")
            raise
    return wrapper

def cache_result(ttl_seconds: int = 300):
    """Decorator to cache method results"""
    cache = {}
    
    def decorator(func):
        @wraps(func)
        async def wrapper(*args, **kwargs):
            key = f"{func.__name__}_{hash(str(args) + str(kwargs))}"
            now = datetime.now()
            
            if key in cache:
                result, timestamp = cache[key]
                if (now - timestamp).total_seconds() < ttl_seconds:
                    return result
            
            result = await func(*args, **kwargs)
            cache[key] = (result, now)
            return result
        return wrapper
    return decorator

class UserService:
    """User service for business logic"""
    
    def __init__(self, repository: UserRepository, logger: Optional[logging.Logger] = None):
        self.repository = repository
        self.logger = logger or logging.getLogger(__name__)
        self._cache: Dict[int, User] = {}
    
    @log_method_call
    async def get_user(self, user_id: int) -> User:
        """Get user by ID"""
        if user_id in self._cache:
            return self._cache[user_id]
        
        user = await self.repository.get_by_id(user_id)
        if not user:
            raise UserNotFoundError(f"User {user_id} not found")
        
        self._cache[user_id] = user
        return user
    
    @log_method_call
    async def create_user(self, name: str, email: str, **metadata) -> User:
        """Create new user"""
        if not validate_email(email):
            raise ValidationError(f"Invalid email: {email}")
        
        existing = await self.repository.find_by_email(email)
        if existing:
            raise ValidationError(f"User with email {email} already exists")
        
        user = User(
            id=0,
            name=name,
            email=email,
            metadata=metadata
        )
        
        saved_user = await self.repository.save(user)
        self._cache[saved_user.id] = saved_user
        
        global user_count
        user_count += 1
        
        self.logger.info(f"Created user: {saved_user.display_name}")
        return saved_user
    
    @log_method_call
    async def update_user(self, user_id: int, **updates) -> User:
        """Update existing user"""
        user = await self.get_user(user_id)
        
        for key, value in updates.items():
            if hasattr(user, key):
                setattr(user, key, value)
        
        if 'email' in updates and not validate_email(updates['email']):
            raise ValidationError(f"Invalid email: {updates['email']}")
        
        updated_user = await self.repository.save(user)
        self._cache[user_id] = updated_user
        
        return updated_user
    
    @log_method_call
    async def delete_user(self, user_id: int) -> bool:
        """Delete user"""
        user = await self.get_user(user_id)
        success = await self.repository.delete(user_id)
        
        if success:
            self._cache.pop(user_id, None)
            global user_count
            user_count -= 1
            self.logger.info(f"Deleted user: {user.display_name}")
        
        return success
    
    @cache_result(ttl_seconds=600)
    async def list_users(self, status: Optional[UserStatus] = None) -> List[User]:
        """List users with optional status filter"""
        users = await self.repository.list_all()
        
        if status:
            users = [user for user in users if user.status == status]
        
        return sorted(users, key=lambda u: u.created_at)
    
    async def search_users(self, query: str) -> List[User]:
        """Search users by name or email"""
        users = await self.repository.list_all()
        query_lower = query.lower()
        
        return [
            user for user in users
            if query_lower in user.name.lower() or query_lower in user.email.lower()
        ]

class UserManager:
    """High-level user manager"""
    
    def __init__(self, service: UserService):
        self.service = service
    
    async def bulk_create_users(self, user_data_list: List[Dict]) -> List[User]:
        """Create multiple users"""
        created_users = []
        
        for user_data in user_data_list:
            try:
                user = await self.service.create_user(**user_data)
                created_users.append(user)
            except (ValidationError, Exception) as e:
                logging.error(f"Failed to create user {user_data}: {e}")
        
        return created_users
    
    async def export_users(self, format: str = 'json') -> str:
        """Export users to string format"""
        users = await self.service.list_users()
        user_dicts = [user.to_dict() for user in users]
        
        if format == 'json':
            return json.dumps(user_dicts, indent=2)
        elif format == 'csv':
            import csv
            import io
            output = io.StringIO()
            if user_dicts:
                fieldnames = user_dicts[0].keys()
                writer = csv.DictWriter(output, fieldnames=fieldnames)
                writer.writeheader()
                writer.writerows(user_dicts)
            return output.getvalue()
        else:
            raise ValueError(f"Unsupported format: {format}")

def initialize_user_system(config: Dict) -> UserService:
    """Initialize the user management system"""
    global is_initialized, user_count
    
    logging.basicConfig(level=config.get('log_level', logging.INFO))
    logger = logging.getLogger(__name__)
    
    repository = MemoryUserRepository()
    service = UserService(repository, logger)
    
    is_initialized = True
    user_count = 0
    
    logger.info("User management system initialized")
    return service

async def main():
    """Main function for testing"""
    config = {
        'log_level': logging.INFO,
        'max_users': MAX_USERS
    }
    
    service = initialize_user_system(config)
    manager = UserManager(service)
    
    # Create test users
    test_users = [
        {'name': 'Alice Johnson', 'email': 'alice@example.com'},
        {'name': 'Bob Smith', 'email': 'bob@example.com'},
        {'name': 'Charlie Brown', 'email': 'charlie@example.com'}
    ]
    
    created_users = await manager.bulk_create_users(test_users)
    print(f"Created {len(created_users)} users")
    
    # List all users
    all_users = await service.list_users()
    print(f"Total users: {len(all_users)}")
    
    # Export users
    json_export = await manager.export_users('json')
    print("Exported users:")
    print(json_export)

if __name__ == "__main__":
    asyncio.run(main())
