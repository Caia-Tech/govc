class UserAuth:
    def __init__(self):
        self.users = {}
    
    def authenticate(self, username, password):
        """Authenticate user with username and password"""
        user = self.users.get(username)
        if user and user["password"] == password:
            return True
        return False
    
    def register_user(self, username, password, email):
        """Register a new user"""
        if username in self.users:
            raise ValueError("User already exists")
        
        self.users[username] = {
            "password": password,
            "email": email
        }
Updated authentication system
