import { Link, Navigate, useNavigate } from 'react-router';

import { client } from '../../api/client';
import { useAuth } from '../../auth/useAuth';
import { AuthForm } from '../../components/AuthForm/AuthForm';
import { unwrap } from '../../hooks/apiError';

export function Login() {
  const { user, signIn } = useAuth();
  const navigate = useNavigate();

  if (user) {
    return <Navigate to="/" replace />;
  }

  const submit = async (login: string, password: string) => {
    const session = unwrap(await client.POST('/api/auth/login', { body: { login, password } }));
    await signIn(session);
    void navigate('/', { replace: true });
  };

  return (
    <AuthForm
      heading="Вход"
      submitLabel="Войти"
      checkPasswordBytes={false}
      newPassword={false}
      footer={<Link to="/register">Регистрация</Link>}
      onSubmit={submit}
    />
  );
}
